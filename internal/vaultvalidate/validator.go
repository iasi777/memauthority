// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultvalidate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iasi777/memauthority/internal/authorityscan"
	"github.com/iasi777/memauthority/internal/runtimeview"
	"github.com/iasi777/memauthority/internal/vaultread"
)

const maxSourceBytes = 2 * 1024 * 1024

var roleSpecs = map[string]struct {
	Filename            string
	Label               string
	DescriptionRequired bool
}{
	"handoff":  {"交接.md", "交接", true},
	"rules":    {"规范.md", "规范", true},
	"progress": {"进度.md", "进度", false},
	"pitfalls": {"踩坑.md", "踩坑", false},
}

var allowedFrontmatter = map[string]bool{
	"项目": true, "创建": true, "最后更新": true, "最后核验": true, "说明": true,
}

var supportRootDirs = map[string]bool{"tools": true}

type Finding struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Field   string `json:"field,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	Valid         bool      `json:"valid"`
	Errors        []Finding `json:"errors"`
}

type roleMeta struct {
	created      string
	lastUpdated  string
	lastVerified string
	datesValid   bool
}

// Validate checks only deterministic Authority format and safety invariants.
// It intentionally does not inspect Git cleanliness, current time, semantic
// memory quality, staleness, or runtime service health.
func Validate(root string) Report {
	report := Report{SchemaVersion: 1, Valid: false, Errors: []Finding{}}
	abs, err := filepath.Abs(root)
	if err != nil {
		return finish(report, Finding{Code: "vault_root", Message: "cannot resolve Vault root"})
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return finish(report, Finding{Code: "vault_root", Message: "Vault root does not exist or cannot be resolved"})
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return finish(report, Finding{Code: "vault_root", Message: "Vault root must be a directory"})
	}

	walkTreeSafety(resolved, &report)

	indexPath := filepath.Join(resolved, "INDEX.yaml")
	raw, ok := readRegular(indexPath, "INDEX.yaml", &report, "index_file")
	if !ok {
		return finish(report)
	}
	if len(raw) > maxSourceBytes {
		add(&report, Finding{Code: "source_too_large", Path: "INDEX.yaml", Message: "INDEX.yaml exceeds the source-size limit"})
		return finish(report)
	}
	if !utf8.Valid(raw) {
		add(&report, Finding{Code: "utf8_invalid", Path: "INDEX.yaml", Message: "INDEX.yaml is not valid UTF-8"})
		return finish(report)
	}
	scanSecret("INDEX.yaml", raw, &report)
	projects, err := vaultread.ParseIndex(raw)
	if err != nil {
		add(&report, Finding{Code: "index_invalid", Path: "INDEX.yaml", Message: err.Error()})
		return finish(report)
	}

	validateRootNamespaces(resolved, projects, &report)

	ids := make([]string, 0, len(projects))
	for id := range projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		validateProject(resolved, projects[id], &report)
	}
	validateRuntimeNamespace(resolved, projects, &report)
	scanMigrationAuthority(resolved, &report)
	if primary, err := os.Lstat(filepath.Join(resolved, "PRIMARY")); err == nil && primary.Mode().IsRegular() {
		if raw, err := os.ReadFile(filepath.Join(resolved, "PRIMARY")); err == nil {
			scanSecret("PRIMARY", raw, &report)
		}
	}
	return finish(report)
}

// ValidateAuthorityRoot is the transaction/serve-compatible fail-closed seam.
// Detailed callers should use Validate to obtain the stable multi-error report.
func ValidateAuthorityRoot(root string) error {
	report := Validate(root)
	if report.Valid {
		return nil
	}
	if len(report.Errors) == 0 {
		return fmt.Errorf("Vault validation failed")
	}
	f := report.Errors[0]
	if f.Path != "" {
		return fmt.Errorf("%s %s: %s", f.Code, f.Path, f.Message)
	}
	return fmt.Errorf("%s: %s", f.Code, f.Message)
}

func finish(report Report, extra ...Finding) Report {
	for _, f := range extra {
		add(&report, f)
	}
	sort.SliceStable(report.Errors, func(i, j int) bool {
		a, b := report.Errors[i], report.Errors[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Message < b.Message
	})
	report.Valid = len(report.Errors) == 0
	return report
}

func add(report *Report, f Finding) { report.Errors = append(report.Errors, f) }

func walkTreeSafety(root string, report *Report) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			rel, _ := filepath.Rel(root, path)
			add(report, Finding{Code: "tree_read", Path: filepath.ToSlash(rel), Message: "cannot inspect Vault tree entry"})
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		slash := filepath.ToSlash(rel)
		if info.Mode()&os.ModeSymlink != 0 {
			add(report, Finding{Code: "symlink_forbidden", Path: slash, Message: "symlinks are not allowed in the validated Vault tree"})
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			add(report, Finding{Code: "non_regular_file", Path: slash, Message: "non-regular files are not allowed in the validated Vault tree"})
		}
		return nil
	})
}

func readRegular(path, rel string, report *Report, code string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		add(report, Finding{Code: code, Path: rel, Message: "required file is missing"})
		return nil, false
	}
	if !info.Mode().IsRegular() {
		add(report, Finding{Code: code, Path: rel, Message: "path must be a regular file"})
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		add(report, Finding{Code: code, Path: rel, Message: "cannot read file"})
		return nil, false
	}
	return raw, true
}

func validateRootNamespaces(root string, projects map[string]vaultread.Project, report *Report) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	roleNames := map[string]bool{}
	for _, spec := range roleSpecs {
		roleNames[spec.Filename] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" || name == "projects" || name == "migrations" || projects[name].ID != "" {
			continue
		}
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || supportRootDirs[name] {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.IsDir() {
			continue
		}
		children, err := os.ReadDir(filepath.Join(root, name))
		if err != nil {
			continue
		}
		for _, child := range children {
			if roleNames[child.Name()] {
				add(report, Finding{Code: "unregistered_project_directory", Path: name, Message: "directory contains a canonical role file but is not registered in INDEX.yaml"})
				break
			}
		}
	}
}

func validateProject(root string, project vaultread.Project, report *Report) {
	projectDir := filepath.Join(root, project.ID)
	relDir := project.ID
	info, err := os.Lstat(projectDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		add(report, Finding{Code: "project_directory", Path: relDir, Message: "registered project directory is missing or invalid"})
		return
	}
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		add(report, Finding{Code: "project_directory", Path: relDir, Message: "cannot read registered project directory"})
		return
	}
	allowed := map[string]bool{}
	for _, spec := range roleSpecs {
		allowed[spec.Filename] = true
	}
	for _, entry := range entries {
		if allowed[entry.Name()] {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.ID, entry.Name()))
		add(report, Finding{Code: "project_namespace", Path: rel, Message: "registered project directories may contain only the four canonical role files"})
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			if raw, err := os.ReadFile(filepath.Join(projectDir, entry.Name())); err == nil {
				scanSecret(rel, raw, report)
			}
		}
	}

	metas := map[string]roleMeta{}
	existing := 0
	validUpdated := true
	for _, role := range []string{"handoff", "rules", "progress", "pitfalls"} {
		spec := roleSpecs[role]
		path := filepath.Join(projectDir, spec.Filename)
		rel := filepath.ToSlash(filepath.Join(project.ID, spec.Filename))
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if role == "handoff" {
				add(report, Finding{Code: "role_required", Path: rel, Message: "handoff role is required"})
			}
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			add(report, Finding{Code: "role_file", Path: rel, Message: "role path must be a regular file"})
			continue
		}
		existing++
		raw, err := os.ReadFile(path)
		if err != nil {
			add(report, Finding{Code: "role_file", Path: rel, Message: "cannot read role file"})
			continue
		}
		meta, ok := validateRole(raw, project.ID, role, rel, report)
		if ok {
			metas[role] = meta
		} else {
			validUpdated = false
		}
	}
	_ = existing

	indexUpdatedOK := validDate(project.LastUpdated)
	if !indexUpdatedOK {
		add(report, Finding{Code: "index_date", Path: "INDEX.yaml", Field: project.ID + ".last_updated", Message: "last_updated must be a valid YYYY-MM-DD string"})
	}
	indexVerifiedOK := project.LastVerified == "未核验" || validDate(project.LastVerified)
	if !indexVerifiedOK {
		add(report, Finding{Code: "index_date", Path: "INDEX.yaml", Field: project.ID + ".last_verified", Message: "last_verified must be YYYY-MM-DD or 未核验"})
	}
	if project.ArchivedAt != nil {
		if _, err := time.Parse(time.RFC3339, *project.ArchivedAt); err != nil {
			add(report, Finding{Code: "index_date", Path: "INDEX.yaml", Field: project.ID + ".archived_at", Message: "archived_at must be RFC3339"})
		}
	}

	if validUpdated && indexUpdatedOK && len(metas) > 0 {
		maxUpdated := ""
		for _, meta := range metas {
			if meta.datesValid && meta.lastUpdated > maxUpdated {
				maxUpdated = meta.lastUpdated
			}
		}
		if maxUpdated != "" && project.LastUpdated != maxUpdated {
			add(report, Finding{Code: "index_last_updated_mismatch", Path: "INDEX.yaml", Field: project.ID + ".last_updated", Message: "INDEX last_updated must equal the maximum role 最后更新"})
		}
	}
	if handoff, ok := metas["handoff"]; ok && indexVerifiedOK && project.LastVerified != handoff.lastVerified {
		add(report, Finding{Code: "index_last_verified_mismatch", Path: "INDEX.yaml", Field: project.ID + ".last_verified", Message: "INDEX last_verified must equal handoff 最后核验"})
	}
}

func validateRole(raw []byte, projectID, role, rel string, report *Report) (roleMeta, bool) {
	meta := roleMeta{}
	if len(raw) > maxSourceBytes {
		add(report, Finding{Code: "source_too_large", Path: rel, Message: "role exceeds the source-size limit"})
		return meta, false
	}
	if !utf8.Valid(raw) {
		add(report, Finding{Code: "utf8_invalid", Path: rel, Message: "role is not valid UTF-8"})
		return meta, false
	}
	scanSecret(rel, raw, report)
	text := strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		add(report, Finding{Code: "frontmatter_required", Path: rel, Line: 1, Message: "role must begin with restricted frontmatter"})
		return meta, false
	}
	close := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			close = i
			break
		}
	}
	if close < 0 {
		add(report, Finding{Code: "frontmatter_required", Path: rel, Line: 1, Message: "role frontmatter is not closed"})
		return meta, false
	}
	values := map[string]string{}
	valueLines := map[string]int{}
	ok := true
	for i := 1; i < close; i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		at := strings.Index(line, ":")
		if at <= 0 {
			add(report, Finding{Code: "frontmatter_invalid", Path: rel, Line: i + 1, Message: "frontmatter entries must use key: value syntax"})
			ok = false
			continue
		}
		key := strings.TrimSpace(line[:at])
		value := strings.TrimSpace(line[at+1:])
		if !allowedFrontmatter[key] {
			add(report, Finding{Code: "frontmatter_field", Path: rel, Field: key, Line: i + 1, Message: "frontmatter field is not allowed"})
			ok = false
			continue
		}
		if _, exists := values[key]; exists {
			add(report, Finding{Code: "frontmatter_duplicate", Path: rel, Field: key, Line: i + 1, Message: "frontmatter field is duplicated"})
			ok = false
			continue
		}
		values[key] = value
		valueLines[key] = i + 1
	}
	required := []string{"项目", "创建", "最后更新", "最后核验"}
	if roleSpecs[role].DescriptionRequired {
		required = append(required, "说明")
	}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			add(report, Finding{Code: "frontmatter_required", Path: rel, Field: key, Message: "required frontmatter field is missing or empty"})
			ok = false
		}
	}
	if v, present := values["说明"]; present && strings.TrimSpace(v) == "" {
		add(report, Finding{Code: "frontmatter_required", Path: rel, Field: "说明", Line: valueLines["说明"], Message: "说明 must be non-empty when present"})
		ok = false
	}
	if v := values["项目"]; v != "" && v != projectID {
		add(report, Finding{Code: "frontmatter_project", Path: rel, Field: "项目", Line: valueLines["项目"], Message: "frontmatter 项目 must match the containing project ID"})
		ok = false
	}

	created, updated, verified := values["创建"], values["最后更新"], values["最后核验"]
	datesOK := true
	if created != "" && !validDate(created) {
		add(report, Finding{Code: "date_invalid", Path: rel, Field: "创建", Line: valueLines["创建"], Message: "创建 must be a valid YYYY-MM-DD date"})
		datesOK = false
		ok = false
	}
	if updated != "" && !validDate(updated) {
		add(report, Finding{Code: "date_invalid", Path: rel, Field: "最后更新", Line: valueLines["最后更新"], Message: "最后更新 must be a valid YYYY-MM-DD date"})
		datesOK = false
		ok = false
	}
	if verified != "" && verified != "未核验" && !validDate(verified) {
		add(report, Finding{Code: "date_invalid", Path: rel, Field: "最后核验", Line: valueLines["最后核验"], Message: "最后核验 must be YYYY-MM-DD or 未核验"})
		datesOK = false
		ok = false
	}
	if datesOK && created != "" && updated != "" && created > updated {
		add(report, Finding{Code: "date_order", Path: rel, Field: "最后更新", Message: "创建 must not be after 最后更新"})
		ok = false
		datesOK = false
	}
	if datesOK && verified != "" && verified != "未核验" && (verified < created || verified > updated) {
		add(report, Finding{Code: "date_order", Path: rel, Field: "最后核验", Message: "verified date must be between 创建 and 最后更新"})
		ok = false
		datesOK = false
	}

	headingLine := 0
	heading := ""
	for i := close + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "# ") {
			headingLine = i + 1
			heading = strings.TrimSpace(strings.TrimPrefix(lines[i], "# "))
			break
		}
	}
	label := roleSpecs[role].Label
	headingOK := heading == label || (strings.HasPrefix(heading, label+" - ") && strings.TrimSpace(strings.TrimPrefix(heading, label+" - ")) != "")
	if !headingOK {
		add(report, Finding{Code: "role_heading", Path: rel, Line: headingLine, Message: "first H1 after frontmatter must match the role label with optional display suffix"})
		ok = false
	}
	meta = roleMeta{created: created, lastUpdated: updated, lastVerified: verified, datesValid: datesOK}
	return meta, ok
}

func validateRuntimeNamespace(root string, projects map[string]vaultread.Project, report *Report) {
	runtimeRoot := filepath.Join(root, "projects")
	info, err := os.Lstat(runtimeRoot)
	if os.IsNotExist(err) {
		for _, p := range projects {
			if p.Lifecycle == "active" && p.RuntimeResource != nil {
				add(report, Finding{Code: "runtime_missing", Path: *p.RuntimeResource, Message: "registered runtime resource is missing"})
			}
		}
		return
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		add(report, Finding{Code: "runtime_namespace", Path: "projects", Message: "runtime namespace must be a directory"})
		return
	}
	dirs, err := os.ReadDir(runtimeRoot)
	if err != nil {
		add(report, Finding{Code: "runtime_namespace", Path: "projects", Message: "cannot read runtime namespace"})
		return
	}
	seen := map[string]bool{}
	for _, entry := range dirs {
		id := entry.Name()
		relDir := filepath.ToSlash(filepath.Join("projects", id))
		project, registered := projects[id]
		info, err := entry.Info()
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			add(report, Finding{Code: "runtime_namespace", Path: relDir, Message: "runtime project entry must be a registered project directory"})
			continue
		}
		if !registered {
			add(report, Finding{Code: "runtime_namespace", Path: relDir, Message: "runtime directory belongs to an unregistered project"})
			continue
		}
		children, err := os.ReadDir(filepath.Join(runtimeRoot, id))
		if err != nil {
			add(report, Finding{Code: "runtime_namespace", Path: relDir, Message: "cannot read runtime project directory"})
			continue
		}
		for _, child := range children {
			rel := filepath.ToSlash(filepath.Join("projects", id, child.Name()))
			if child.Name() != "runtime.yaml" {
				add(report, Finding{Code: "runtime_namespace", Path: rel, Message: "runtime project directory may contain only runtime.yaml"})
				continue
			}
			seen[id] = true
			raw, ok := readRegular(filepath.Join(runtimeRoot, id, "runtime.yaml"), rel, report, "runtime_file")
			if !ok {
				continue
			}
			if len(raw) > maxSourceBytes {
				add(report, Finding{Code: "source_too_large", Path: rel, Message: "runtime exceeds the source-size limit"})
				continue
			}
			if !utf8.Valid(raw) {
				add(report, Finding{Code: "utf8_invalid", Path: rel, Message: "runtime is not valid UTF-8"})
				continue
			}
			scanSecret(rel, raw, report)
			if err := runtimeview.ValidateManifest(raw, id); err != nil {
				add(report, Finding{Code: "runtime_invalid", Path: rel, Message: err.Error()})
			}
			if project.Lifecycle == "active" && project.RuntimeResource == nil {
				add(report, Finding{Code: "runtime_orphan", Path: rel, Message: "active project runtime file exists but is not registered in INDEX.yaml"})
			}
		}
	}
	for id, p := range projects {
		if p.Lifecycle == "active" && p.RuntimeResource != nil && !seen[id] {
			add(report, Finding{Code: "runtime_missing", Path: *p.RuntimeResource, Message: "registered runtime resource is missing"})
		}
	}
}

func scanMigrationAuthority(root string, report *Report) {
	base := filepath.Join(root, "migrations")
	info, err := os.Lstat(base)
	if os.IsNotExist(err) {
		return
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		add(report, Finding{Code: "migration_namespace", Path: "migrations", Message: "migrations namespace must be a directory"})
		return
	}
	_ = filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		scanSecret(filepath.ToSlash(rel), raw, report)
		return nil
	})
}

func scanSecret(rel string, raw []byte, report *Report) {
	for _, line := range authorityscan.HighConfidenceSecretLines(raw) {
		add(report, Finding{Code: "secret_detected", Path: rel, Line: line, Message: "high-confidence secret material is not allowed in Authority"})
	}
}

func validDate(value string) bool {
	if len(value) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
