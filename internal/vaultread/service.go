// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/iasi777/v-memory/internal/checklist"
	"github.com/iasi777/v-memory/internal/gitview"
	"github.com/iasi777/v-memory/internal/textcanon"
	"gopkg.in/yaml.v3"
)

const (
	indexSchemaVersion = "4"
	maxPageBytes       = 16 * 1024
	maxSourceBytes     = 2 * 1024 * 1024
)

var (
	roleFiles = map[string]string{
		"handoff":  "交接.md",
		"rules":    "规范.md",
		"progress": "进度.md",
		"pitfalls": "踩坑.md",
	}
	headingRE   = regexp.MustCompile(`^(#{1,6})[ \t]+(.*?)[ \t]*$`)
	projectIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

const memoryURIHint = "expected format memory://projects/{project_id}/{role}; role must be one of: handoff, rules, progress, pitfalls"

type Project struct {
	ID              string
	Description     string
	Lifecycle       string
	Aliases         []string
	RuntimeResource *string
	LocalPath       *string
	ArchivedAt      *string
	ArchiveReason   *string
	LastUpdated     string
	LastVerified    string
}

type Service struct {
	root        string
	projects    map[string]Project
	head        string
	commitBound bool
	projection  *projection
}

type OpenOptions struct {
	ProjectionPath string
	ForceRebuild   bool
	// SourceCommit pins the reader to one immutable Git commit. When empty,
	// OpenWithOptions resolves the current HEAD once and owns that snapshot.
	SourceCommit string
}

type ReadArgs struct {
	URI                      string `json:"uri"`
	Heading                  string `json:"heading,omitempty"`
	LineStart                *int   `json:"line_start,omitempty"`
	LineEnd                  *int   `json:"line_end,omitempty"`
	Cursor                   string `json:"cursor,omitempty"`
	ExpectedResourceRevision string `json:"expected_resource_revision,omitempty"`
}

type cursorState struct {
	URI       string `json:"u"`
	Heading   string `json:"h,omitempty"`
	LineStart *int   `json:"s,omitempty"`
	LineEnd   *int   `json:"e,omitempty"`
	Offset    int    `json:"o"`
}

func Open(root string) (*Service, error) {
	return OpenWithOptions(root, OpenOptions{})
}

func OpenWithOptions(root string, options OpenOptions) (*Service, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root symlinks: %w", err)
	}
	commitBound := strings.TrimSpace(options.SourceCommit) != ""
	var head string
	var raw []byte
	if commitBound {
		head, err = gitview.ResolveCommit(resolved, options.SourceCommit)
		if err != nil {
			return nil, err
		}
		raw, err = gitview.ReadFile(resolved, head, "INDEX.yaml")
		if err != nil {
			return nil, fmt.Errorf("read INDEX.yaml at owned commit: %w", err)
		}
	} else {
		raw, err = os.ReadFile(filepath.Join(resolved, "INDEX.yaml"))
		if err != nil {
			return nil, fmt.Errorf("read INDEX.yaml: %w", err)
		}
		head, err = gitview.ResolveCommit(resolved, "HEAD")
		if err != nil {
			return nil, err
		}
	}
	projects, err := parseIndex(raw)
	if err != nil {
		return nil, err
	}
	service := &Service{root: resolved, projects: projects, head: head, commitBound: commitBound}
	if options.ProjectionPath != "" {
		projection, err := openProjection(service, options.ProjectionPath, options.ForceRebuild)
		if err != nil {
			return nil, err
		}
		service.projection = projection
	}
	return service, nil
}

func (s *Service) Close() error {
	if s.projection == nil || s.projection.db == nil {
		return nil
	}
	return s.projection.db.Close()
}

func (s *Service) SourceCommit() string {
	return s.head
}

// IndexHead returns the rebuildable projection head, or an empty string when no projection is configured.
func (s *Service) IndexHead() string {
	if s.projection == nil {
		return ""
	}
	return s.projection.head
}

// ResolveRoleFile returns a path-confined existing Authority role file for internal services.
func (s *Service) ResolveRoleFile(projectID, role string) (string, error) {
	if _, ok := s.projects[projectID]; !ok {
		return "", fmt.Errorf("unknown project")
	}
	filename, ok := roleFiles[role]
	if !ok {
		return "", fmt.Errorf("unknown role")
	}
	return safeExistingFile(s.root, filepath.Join(projectID, filename))
}

func (s *Service) readRole(projectID, role string) ([]byte, error) {
	if _, ok := s.projects[projectID]; !ok {
		return nil, fmt.Errorf("unknown project")
	}
	filename, ok := roleFiles[role]
	if !ok {
		return nil, fmt.Errorf("unknown role")
	}
	var raw []byte
	var err error
	if s.commitBound {
		raw, err = gitview.ReadFile(s.root, s.head, filepath.ToSlash(filepath.Join(projectID, filename)))
		if err != nil {
			return nil, fmt.Errorf("read role at owned commit: %w", err)
		}
	} else {
		path, pathErr := safeExistingFile(s.root, filepath.Join(projectID, filename))
		if pathErr != nil {
			return nil, pathErr
		}
		raw, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read role: %w", err)
		}
	}
	if len(raw) > maxSourceBytes {
		return nil, fmt.Errorf("resource exceeds source limit")
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("resource is not valid UTF-8")
	}
	return raw, nil
}

func (s *Service) resourceURIs(projectID string) map[string]string {
	result := map[string]string{}
	for role := range roleFiles {
		if _, err := s.readRole(projectID, role); err == nil {
			result[role] = fmt.Sprintf("memory://projects/%s/%s", projectID, role)
		}
	}
	return result
}

func (s *Service) Projects() []Project {
	ids := make([]string, 0, len(s.projects))
	for id := range s.projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Project, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.projects[id])
	}
	return out
}

func (s *Service) Route(query string) map[string]any {
	key := textcanon.FoldKey(query)
	if key == "" {
		return map[string]any{"status": "not_found", "project_id": nil, "reasons": []string{"query:" + query}}
	}
	type routeCandidate struct {
		project Project
		reasons map[string]struct{}
	}
	candidates := map[string]*routeCandidate{}
	ids := make([]string, 0, len(s.projects))
	for id := range s.projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := s.projects[id]
		add := func(reason string) {
			item := candidates[p.ID]
			if item == nil {
				item = &routeCandidate{project: p, reasons: map[string]struct{}{}}
				candidates[p.ID] = item
			}
			item.reasons[reason] = struct{}{}
		}
		if textcanon.FoldKey(p.ID) == key {
			add("query:" + p.ID)
		}
		for _, alias := range p.Aliases {
			if textcanon.FoldKey(alias) == key {
				add("alias:" + alias)
			}
		}
	}
	if len(candidates) == 0 {
		return map[string]any{"status": "not_found", "project_id": nil, "reasons": []string{"query:" + query}}
	}
	if len(candidates) > 1 {
		projectIDs := make([]string, 0, len(candidates))
		for id := range candidates {
			projectIDs = append(projectIDs, id)
		}
		sort.Strings(projectIDs)
		return map[string]any{"status": "ambiguous", "project_ids": projectIDs, "reasons": []string{"query:" + query}}
	}
	var candidate *routeCandidate
	for _, item := range candidates {
		candidate = item
	}
	reasons := make([]string, 0, len(candidate.reasons))
	for reason := range candidate.reasons {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	p := candidate.project
	if p.Lifecycle == "archived" {
		var reason, archived any
		if p.ArchiveReason != nil {
			reason = *p.ArchiveReason
		}
		if p.ArchivedAt != nil {
			archived = *p.ArchivedAt
		}
		return map[string]any{"status": "archived", "project_id": p.ID, "resource_uris": s.resourceURIs(p.ID), "reasons": reasons, "archive_reason": reason, "archived_at": archived}
	}
	return map[string]any{"status": "unique", "project_id": p.ID, "resource_uris": s.resourceURIs(p.ID), "reasons": reasons, "archive_reason": nil, "archived_at": nil}
}

func (s *Service) Read(args ReadArgs) map[string]any {
	result, err := s.read(args)
	if err == nil {
		return result
	}
	var conflict *revisionConflict
	if errors.As(err, &conflict) {
		return map[string]any{
			"status": "error", "code": "conflict", "message": conflict.Error(),
			"current_revision": conflict.Current, "revision": nil, "content": nil,
		}
	}
	return map[string]any{
		"status": "error", "code": "read_failed", "message": err.Error(),
		"revision": nil, "content": nil,
	}
}

type revisionConflict struct{ Current string }

func (e *revisionConflict) Error() string { return "resource revision does not match current content" }

func (s *Service) read(args ReadArgs) (map[string]any, error) {
	projectID, role, err := parseMemoryURI(args.URI)
	if err != nil {
		return nil, err
	}
	raw, err := s.readRole(projectID, role)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(raw)
	revision := "sha256:" + hex.EncodeToString(h[:])
	if args.ExpectedResourceRevision != "" && args.ExpectedResourceRevision != revision {
		return nil, &revisionConflict{Current: revision}
	}

	state := cursorState{URI: args.URI, Heading: args.Heading, LineStart: args.LineStart, LineEnd: args.LineEnd}
	if args.Cursor != "" {
		if args.Heading != "" || args.LineStart != nil || args.LineEnd != nil {
			return nil, fmt.Errorf("cursor cannot be combined with another selector")
		}
		state, err = decodeCursor(args.Cursor)
		if err != nil || state.URI != args.URI || state.Offset < 0 {
			return nil, fmt.Errorf("invalid cursor")
		}
	}
	selected, lineStart, lineEnd, err := selectContent(raw, state.Heading, state.LineStart, state.LineEnd)
	if err != nil {
		return nil, err
	}
	if state.Offset > len(selected) {
		return nil, fmt.Errorf("invalid cursor offset")
	}
	initialOffset := state.Offset
	page := selected[state.Offset:]
	end := len(page)
	if end > maxPageBytes {
		end = maxPageBytes
		for end > 0 && !utf8.Valid(page[:end]) {
			end--
		}
	}
	content := string(page[:end])
	nextOffset := state.Offset + end
	truncated := nextOffset < len(selected)
	var cursor any
	if truncated {
		state.Offset = nextOffset
		encoded, err := encodeCursor(state)
		if err != nil {
			return nil, err
		}
		cursor = encoded
	} else {
		cursor = nil
	}
	sourceCommit := s.head
	if !s.commitBound {
		if current, headErr := gitview.ResolveCommit(s.root, "HEAD"); headErr == nil {
			sourceCommit = current
		}
	}
	project := s.projects[projectID]
	lastUpdated, lastVerified := readFrontmatterDates(raw)
	if lastUpdated == "" {
		lastUpdated = project.LastUpdated
	}
	if lastVerified == "" {
		lastVerified = project.LastVerified
	}
	staleness := "current"
	if project.Lifecycle == "archived" {
		staleness = "archived"
	} else if lastVerified == "未核验" {
		staleness = "review"
	}
	result := map[string]any{
		"status": "ok", "uri": args.URI, "project_id": projectID, "role": role,
		"content": content, "revision": revision, "content_hash": revision,
		"source_commit": sourceCommit, "line_start": lineStart, "line_end": lineEnd,
		"truncated": truncated, "cursor": cursor, "last_updated": lastUpdated, "last_verified": lastVerified, "staleness": staleness,
	}
	if role == "handoff" && initialOffset == 0 && checklistSelectorEligible(state) {
		result["checklist_items"] = checklist.ParseHandoff(raw, revision)
	}
	return result, nil
}

func checklistSelectorEligible(state cursorState) bool {
	if state.LineStart != nil || state.LineEnd != nil {
		return false
	}
	return state.Heading == "" || strings.TrimSpace(state.Heading) == checklist.TodoHeading
}

func readFrontmatterDates(raw []byte) (string, string) {
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") {
		return "", ""
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", ""
	}
	front := text[4 : 4+end]
	var updated, verified string
	for _, line := range strings.Split(front, "\n") {
		if strings.HasPrefix(line, "最后更新:") {
			updated = strings.TrimSpace(strings.TrimPrefix(line, "最后更新:"))
		}
		if strings.HasPrefix(line, "最后核验:") {
			verified = strings.TrimSpace(strings.TrimPrefix(line, "最后核验:"))
		}
	}
	return updated, verified
}

func parseMemoryURI(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "memory" || u.Host != "projects" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("invalid memory URI; %s", memoryURIHint)
	}
	escaped := strings.TrimPrefix(u.EscapedPath(), "/")
	parts := strings.Split(escaped, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid memory URI path; %s", memoryURIHint)
	}
	project, err := url.PathUnescape(parts[0])
	if err != nil || !validProjectID(project) {
		return "", "", fmt.Errorf("invalid project path; %s", memoryURIHint)
	}
	role, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid role path; %s", memoryURIHint)
	}
	if _, ok := roleFiles[role]; !ok {
		return "", "", fmt.Errorf("invalid role; %s", memoryURIHint)
	}
	return project, role, nil
}

func safeExistingFile(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("absolute path is not allowed")
	}
	for _, p := range strings.Split(filepath.ToSlash(relative), "/") {
		if p == "" || p == "." || p == ".." {
			return "", fmt.Errorf("unsafe relative path")
		}
	}
	candidate := filepath.Join(root, relative)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve role path: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("resolved path escapes vault root")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("role path is not a regular file")
	}
	return resolved, nil
}

func selectContent(raw []byte, heading string, lineStart, lineEnd *int) ([]byte, int, int, error) {
	if heading != "" && (lineStart != nil || lineEnd != nil) {
		return nil, 0, 0, fmt.Errorf("heading and line range cannot be combined")
	}
	lines, starts := splitLines(raw)
	if heading != "" {
		start := -1
		level := 0
		for i, line := range lines {
			m := headingRE.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
			if m != nil && strings.TrimSpace(m[2]) == heading {
				start = i
				level = len(m[1])
				break
			}
		}
		if start < 0 {
			return nil, 0, 0, fmt.Errorf("heading not found")
		}
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			m := headingRE.FindStringSubmatch(strings.TrimRight(lines[i], "\r\n"))
			if m != nil && len(m[1]) <= level {
				end = i
				break
			}
		}
		return raw[starts[start]:starts[end]], start + 1, end, nil
	}
	if lineStart != nil || lineEnd != nil {
		if lineStart == nil || lineEnd == nil || *lineStart < 1 || *lineEnd < *lineStart || *lineEnd > len(lines) {
			return nil, 0, 0, fmt.Errorf("invalid line range")
		}
		return raw[starts[*lineStart-1]:starts[*lineEnd]], *lineStart, *lineEnd, nil
	}
	return raw, 1, len(lines), nil
}

func splitLines(raw []byte) ([]string, []int) {
	if len(raw) == 0 {
		return []string{""}, []int{0, 0}
	}
	lines := strings.SplitAfter(string(raw), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	starts := make([]int, len(lines)+1)
	pos := 0
	for i, line := range lines {
		starts[i] = pos
		pos += len([]byte(line))
	}
	starts[len(lines)] = pos
	return lines, starts
}

func encodeCursor(state cursorState) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeCursor(encoded string) (cursorState, error) {
	var state cursorState
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	return state, nil
}

func ValidProjectID(v string) bool {
	if !projectIDRE.MatchString(v) {
		return false
	}
	lower := strings.ToLower(v)
	if lower == "projects" || lower == "migrations" || lower == "primary" {
		return false
	}
	if lower == "con" || lower == "prn" || lower == "aux" || lower == "nul" {
		return false
	}
	if len(lower) == 4 && (strings.HasPrefix(lower, "com") || strings.HasPrefix(lower, "lpt")) && lower[3] >= '1' && lower[3] <= '9' {
		return false
	}
	return true
}

func validProjectID(v string) bool { return ValidProjectID(v) }

func ParseIndex(raw []byte) (map[string]Project, error) { return parseIndex(raw) }

func parseIndex(raw []byte) (map[string]Project, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse INDEX.yaml: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("INDEX.yaml must be a mapping")
	}
	root, err := mapping(doc.Content[0])
	if err != nil {
		return nil, err
	}
	if len(root) != 2 || root["schema_version"] == nil || root["projects"] == nil {
		return nil, fmt.Errorf("INDEX.yaml requires only schema_version and projects")
	}
	version := root["schema_version"]
	if version.Kind != yaml.ScalarNode || version.Tag != "!!int" || version.Value != indexSchemaVersion {
		return nil, fmt.Errorf("unsupported INDEX schema_version")
	}
	projectsNode := root["projects"]
	if projectsNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("INDEX projects must be a mapping")
	}
	pairs, err := mappingPairs(projectsNode)
	if err != nil {
		return nil, err
	}
	projects := make(map[string]Project, len(pairs))
	casefoldIDs := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		idNode, valueNode := pair[0], pair[1]
		if idNode.Kind != yaml.ScalarNode || idNode.Tag != "!!str" || !ValidProjectID(idNode.Value) {
			return nil, fmt.Errorf("invalid project ID")
		}
		fold := strings.ToLower(idNode.Value)
		if previous, ok := casefoldIDs[fold]; ok && previous != idNode.Value {
			return nil, fmt.Errorf("project IDs collide under ASCII case folding")
		}
		casefoldIDs[fold] = idNode.Value
		p, err := parseProject(idNode.Value, valueNode)
		if err != nil {
			return nil, err
		}
		projects[p.ID] = p
	}
	return projects, nil
}

func parseProject(id string, node *yaml.Node) (Project, error) {
	m, err := mapping(node)
	if err != nil {
		return Project{}, err
	}
	required := []string{"description", "lifecycle", "aliases", "runtime_resource", "last_updated", "last_verified"}
	allowed := map[string]bool{"description": true, "lifecycle": true, "aliases": true, "runtime_resource": true, "last_updated": true, "last_verified": true, "local_path": true, "archived_at": true, "archive_reason": true}
	for key := range m {
		if !allowed[key] {
			return Project{}, fmt.Errorf("invalid INDEX field for %s: %s", id, key)
		}
	}
	for _, key := range required {
		if m[key] == nil {
			return Project{}, fmt.Errorf("missing INDEX field for %s: %s", id, key)
		}
	}
	p := Project{ID: id}
	if p.Description, err = textScalar(m["description"], "description"); err != nil {
		return Project{}, err
	}
	if p.Lifecycle, err = textScalar(m["lifecycle"], "lifecycle"); err != nil {
		return Project{}, err
	}
	if p.Lifecycle != "active" && p.Lifecycle != "archived" {
		return Project{}, fmt.Errorf("invalid lifecycle for %s", id)
	}
	if p.Aliases, err = stringList(m["aliases"], "aliases"); err != nil {
		return Project{}, err
	}
	if p.RuntimeResource, err = optionalText(m["runtime_resource"], "runtime_resource"); err != nil {
		return Project{}, err
	}
	if p.LocalPath, err = optionalText(m["local_path"], "local_path"); err != nil {
		return Project{}, err
	}
	if p.ArchivedAt, err = optionalText(m["archived_at"], "archived_at"); err != nil {
		return Project{}, err
	}
	if p.ArchiveReason, err = optionalText(m["archive_reason"], "archive_reason"); err != nil {
		return Project{}, err
	}
	if p.LastUpdated, err = textScalar(m["last_updated"], "last_updated"); err != nil {
		return Project{}, err
	}
	if p.LastVerified, err = textScalar(m["last_verified"], "last_verified"); err != nil {
		return Project{}, err
	}
	if p.RuntimeResource != nil && *p.RuntimeResource != "projects/"+id+"/runtime.yaml" {
		return Project{}, fmt.Errorf("invalid runtime_resource for %s", id)
	}
	if p.Lifecycle == "active" && (p.ArchivedAt != nil || p.ArchiveReason != nil) {
		return Project{}, fmt.Errorf("active project has archive fields")
	}
	if p.Lifecycle == "archived" && (p.ArchivedAt == nil || p.ArchiveReason == nil || p.RuntimeResource != nil) {
		return Project{}, fmt.Errorf("invalid archived project fields")
	}
	return p, nil
}

func mapping(node *yaml.Node) (map[string]*yaml.Node, error) {
	pairs, err := mappingPairs(node)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*yaml.Node, len(pairs))
	for _, pair := range pairs {
		key := pair[0]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, fmt.Errorf("mapping key must be text")
		}
		out[key.Value] = pair[1]
	}
	return out, nil
}

func mappingPairs(node *yaml.Node) ([][2]*yaml.Node, error) {
	if node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return nil, fmt.Errorf("expected YAML mapping")
	}
	seen := map[string]bool{}
	out := make([][2]*yaml.Node, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		key := k.Value
		if seen[key] {
			return nil, fmt.Errorf("duplicate YAML key: %s", key)
		}
		seen[key] = true
		out = append(out, [2]*yaml.Node{k, v})
	}
	return out, nil
}

func textScalar(node *yaml.Node, field string) (string, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" || strings.TrimSpace(node.Value) == "" || strings.ContainsAny(node.Value, "\r\n") {
		return "", fmt.Errorf("%s must be non-empty single-line text", field)
	}
	return strings.TrimSpace(node.Value), nil
}

func optionalText(node *yaml.Node, field string) (*string, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil, nil
	}
	v, err := textScalar(node, field)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func stringList(node *yaml.Node, field string) ([]string, error) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a list", field)
	}
	out := make([]string, 0, len(node.Content))
	seen := map[string]bool{}
	for _, n := range node.Content {
		v, err := textScalar(n, field)
		if err != nil {
			return nil, err
		}
		if seen[v] {
			return nil, fmt.Errorf("duplicate alias")
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}
