// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/iasi777/v-memory/internal/gitview"
	_ "modernc.org/sqlite"
)

const projectionSchemaVersion = "1"

var projectionRoleOrder = []string{"handoff", "rules", "progress", "pitfalls"}

type projection struct {
	db   *sql.DB
	path string
	head string
}

type SearchArgs struct {
	Query        string `json:"query" jsonschema:"Text to search for in indexed memory sections; must be non-empty."`
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project id to search when cross_project is false."`
	CrossProject bool   `json:"cross_project,omitempty" jsonschema:"Set true to search all registered projects; otherwise project_id is required."`
}

type indexedSection struct {
	ProjectID        string
	Role             string
	Heading          string
	LineStart        int
	LineEnd          int
	Content          string
	SectionHash      string
	ResourceRevision string
	ResourceURI      string
	SourceRevision   string
}

func openProjection(service *Service, path string, force bool) (*projection, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve projection path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("create projection directory: %w", err)
	}
	if !force {
		if p, err := openValidProjection(abs, service.head); err == nil {
			return p, nil
		}
	}
	if err := rebuildProjection(service, abs); err != nil {
		return nil, err
	}
	p, err := openValidProjection(abs, service.head)
	if err != nil {
		return nil, fmt.Errorf("open rebuilt projection: %w", err)
	}
	return p, nil
}

func openValidProjection(path, expectedHead string) (*projection, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	closeOnError := func(err error) (*projection, error) {
		_ = db.Close()
		return nil, err
	}
	var quick string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&quick); err != nil || quick != "ok" {
		if err == nil {
			err = fmt.Errorf("projection quick_check: %s", quick)
		}
		return closeOnError(err)
	}
	var version, head string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		return closeOnError(err)
	}
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='index_head'`).Scan(&head); err != nil {
		return closeOnError(err)
	}
	if version != projectionSchemaVersion || head != expectedHead {
		return closeOnError(fmt.Errorf("projection is stale"))
	}
	return &projection{db: db, path: path, head: head}, nil
}

func rebuildProjection(service *Service, path string) error {
	// Projection files are rebuildable state only. Never remove or mutate anything under service.root.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale projection%s: %w", suffix, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("create projection: %w", err)
	}
	db.SetMaxOpenConns(1)
	ok := false
	defer func() {
		_ = db.Close()
		if !ok {
			_ = os.Remove(path)
			_ = os.Remove(path + "-journal")
			_ = os.Remove(path + "-wal")
			_ = os.Remove(path + "-shm")
		}
	}()
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL;`); err != nil {
		return fmt.Errorf("configure projection: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin projection rebuild: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	schema := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE sections (
            id INTEGER PRIMARY KEY,
            project_id TEXT NOT NULL,
            role TEXT NOT NULL,
            heading TEXT NOT NULL,
            line_start INTEGER NOT NULL,
            line_end INTEGER NOT NULL,
            content TEXT NOT NULL,
            section_hash TEXT NOT NULL,
            resource_revision TEXT NOT NULL,
            resource_uri TEXT NOT NULL,
            source_revision TEXT NOT NULL,
            search_text TEXT NOT NULL
        )`,
		`CREATE INDEX sections_project_line ON sections(project_id, role, line_start)`,
		`CREATE VIRTUAL TABLE sections_fts USING fts5(search_text, content='sections', content_rowid='id', tokenize='trigram')`,
	}
	for _, stmt := range schema {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("create projection schema: %w", err)
		}
	}
	sections, err := collectSections(service)
	if err != nil {
		return err
	}
	insert, err := tx.Prepare(`INSERT INTO sections(project_id, role, heading, line_start, line_end, content, section_hash, resource_revision, resource_uri, source_revision, search_text) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare projection insert: %w", err)
	}
	for _, section := range sections {
		if _, err := insert.Exec(section.ProjectID, section.Role, section.Heading, section.LineStart, section.LineEnd, section.Content, section.SectionHash, section.ResourceRevision, section.ResourceURI, section.SourceRevision, section.Content); err != nil {
			_ = insert.Close()
			return fmt.Errorf("insert projection section: %w", err)
		}
	}
	if err := insert.Close(); err != nil {
		return fmt.Errorf("close projection insert: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO sections_fts(sections_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("build projection FTS: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('schema_version', ?), ('index_head', ?)`, projectionSchemaVersion, service.head); err != nil {
		return fmt.Errorf("write projection metadata: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit projection rebuild: %w", err)
	}
	rollback = false
	if err := db.Close(); err != nil {
		return fmt.Errorf("close projection after rebuild: %w", err)
	}
	ok = true
	return nil
}

func collectSections(service *Service) ([]indexedSection, error) {
	ids := make([]string, 0, len(service.projects))
	for id := range service.projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []indexedSection
	for _, projectID := range ids {
		for _, role := range projectionRoleOrder {
			filename := roleFiles[role]
			var raw []byte
			var err error
			if service.commitBound {
				raw, err = gitview.ReadFile(service.root, service.head, filepath.ToSlash(filepath.Join(projectID, filename)))
				if err != nil {
					if role != "handoff" {
						var exitErr *exec.ExitError
						if errors.As(err, &exitErr) {
							continue
						}
					}
					return nil, fmt.Errorf("read indexed role at owned commit: %w", err)
				}
			} else {
				candidate := filepath.Join(service.root, projectID, filename)
				if _, statErr := os.Lstat(candidate); statErr != nil {
					if errors.Is(statErr, os.ErrNotExist) && role != "handoff" {
						continue
					}
					return nil, fmt.Errorf("inspect %s/%s: %w", projectID, role, statErr)
				}
				path, pathErr := safeExistingFile(service.root, filepath.Join(projectID, filename))
				if pathErr != nil {
					return nil, pathErr
				}
				raw, err = os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("read indexed role: %w", err)
				}
			}
			if len(raw) > maxSourceBytes || !utf8.Valid(raw) {
				return nil, fmt.Errorf("invalid indexed role %s/%s", projectID, role)
			}
			sum := sha256.Sum256(raw)
			revision := "sha256:" + hex.EncodeToString(sum[:])
			sections := splitIndexedSections(raw, projectID, role, revision, service.head)
			out = append(out, sections...)
		}
	}
	return out, nil
}

func splitIndexedSections(raw []byte, projectID, role, revision, sourceRevision string) []indexedSection {
	lines, starts := splitLines(raw)
	type heading struct {
		index int
		level int
		title string
		path  string
	}
	var headings []heading
	stack := make([]string, 6)
	for i, line := range lines {
		match := headingRE.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
		if match == nil {
			continue
		}
		level := len(match[1])
		title := strings.TrimSpace(match[2])
		if title == "" {
			continue
		}
		stack[level-1] = title
		for j := level; j < len(stack); j++ {
			stack[j] = ""
		}
		parts := make([]string, 0, level)
		for j := 0; j < level; j++ {
			if stack[j] != "" {
				parts = append(parts, stack[j])
			}
		}
		headings = append(headings, heading{index: i, level: level, title: title, path: strings.Join(parts, " / ")})
	}
	result := make([]indexedSection, 0, len(headings)+1)
	if len(headings) > 0 && headings[0].index > 0 {
		contentBytes := raw[:starts[headings[0].index]]
		sectionSum := sha256.Sum256(contentBytes)
		result = append(result, indexedSection{
			ProjectID:        projectID,
			Role:             role,
			Heading:          "",
			LineStart:        1,
			LineEnd:          headings[0].index,
			Content:          string(contentBytes),
			SectionHash:      "sha256:" + hex.EncodeToString(sectionSum[:]),
			ResourceRevision: revision,
			ResourceURI:      fmt.Sprintf("memory://projects/%s/%s", projectID, role),
			SourceRevision:   sourceRevision,
		})
	}
	for i, h := range headings {
		endIndex := len(lines)
		if i+1 < len(headings) {
			endIndex = headings[i+1].index
		}
		if h.index >= len(starts)-1 || endIndex >= len(starts) {
			continue
		}
		contentBytes := raw[starts[h.index]:starts[endIndex]]
		sectionSum := sha256.Sum256(contentBytes)
		result = append(result, indexedSection{
			ProjectID:        projectID,
			Role:             role,
			Heading:          h.path,
			LineStart:        h.index + 1,
			LineEnd:          endIndex,
			Content:          string(contentBytes),
			SectionHash:      "sha256:" + hex.EncodeToString(sectionSum[:]),
			ResourceRevision: revision,
			ResourceURI:      fmt.Sprintf("memory://projects/%s/%s", projectID, role),
			SourceRevision:   sourceRevision,
		})
	}
	return result
}

func (s *Service) Search(args SearchArgs) map[string]any {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return searchError("search_failed", "memory_search query must be non-empty")
	}
	if !args.CrossProject && args.ProjectID == "" {
		return searchError("project_required", "memory_search requires project_id unless cross_project=true")
	}
	if args.ProjectID != "" {
		if _, ok := s.projects[args.ProjectID]; !ok {
			return searchError("project_not_found", "unknown project")
		}
	}
	if s.projection == nil || s.projection.db == nil {
		return searchError("index_unavailable", "read projection is not configured")
	}
	matchType := "fts5_trigram"
	useFTS := utf8.RuneCountInString(query) >= 3
	if !useFTS {
		matchType = "like_short_query"
	}
	results, err := s.projection.search(query, args.ProjectID, args.CrossProject, useFTS, matchType)
	if err != nil {
		return searchError("search_failed", err.Error())
	}
	truncated := len(results) > 50
	if truncated {
		results = results[:50]
	}
	var project any = args.ProjectID
	if args.CrossProject {
		project = nil
	}
	return map[string]any{
		"status": "ok", "code": nil, "message": nil,
		"project_id": project, "results": results,
		"index_head": s.projection.head, "match_type": matchType,
		"truncated": truncated, "cursor": nil,
	}
}

func searchError(code, message string) map[string]any {
	return map[string]any{
		"status": "error", "code": code, "message": message,
		"project_id": nil, "results": nil, "index_head": nil,
		"match_type": nil, "truncated": nil, "cursor": nil,
	}
}

func (p *projection) search(query, projectID string, crossProject, useFTS bool, matchType string) ([]map[string]any, error) {
	var rows *sql.Rows
	var err error
	base := `SELECT s.project_id, s.role, s.heading, s.line_start, s.line_end, s.content, s.section_hash, s.resource_revision, s.resource_uri, s.source_revision
             FROM sections s `
	if useFTS {
		phrase := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
		base += `JOIN sections_fts f ON f.rowid=s.id WHERE sections_fts MATCH ?`
		args := []any{phrase}
		if !crossProject {
			base += ` AND s.project_id=?`
			args = append(args, projectID)
		}
		base += ` ORDER BY s.project_id, s.role, s.line_start LIMIT 51`
		rows, err = p.db.Query(base, args...)
	} else {
		base += `WHERE instr(s.search_text, ?) > 0`
		args := []any{query}
		if !crossProject {
			base += ` AND s.project_id=?`
			args = append(args, projectID)
		}
		base += ` ORDER BY s.project_id, s.role, s.line_start LIMIT 51`
		rows, err = p.db.Query(base, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]map[string]any, 0)
	for rows.Next() {
		var project, role, heading, content, sectionHash, revision, uri, source string
		var lineStart, lineEnd int
		if err := rows.Scan(&project, &role, &heading, &lineStart, &lineEnd, &content, &sectionHash, &revision, &uri, &source); err != nil {
			return nil, err
		}
		results = append(results, map[string]any{
			"project_id": project, "role": role, "heading": heading,
			"line_start": lineStart, "line_end": lineEnd,
			"snippet": projectionSnippet(content), "section_hash": sectionHash,
			"resource_revision": revision, "content_hash": revision,
			"resource_uri": uri, "source_revision": source,
			"match_type": matchType,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func projectionSnippet(content string) string {
	const maxRunes = 400
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + " …"
}
