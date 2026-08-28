// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package runtimeview

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RuntimeClosedValues exposes the validator's closed value sets without
// duplicating them in MCP metadata or tests. Returned slices are sorted copies.
type RuntimeClosedValues struct {
	EnvironmentKinds []string
	Purposes         []string
	Capabilities     []string
	SupervisorKinds  []string
	RelationKinds    []string
	CheckKinds       []string
}

func ClosedValues() RuntimeClosedValues {
	return RuntimeClosedValues{
		EnvironmentKinds: sortedSet(environmentKinds),
		Purposes:         sortedSet(purposes),
		Capabilities:     sortedSet(capabilities),
		SupervisorKinds:  sortedSet(supervisorKinds),
		RelationKinds:    sortedSet(relationKinds),
		CheckKinds:       sortedSet(checkKinds),
	}
}

type ValidationFinding struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Field   string `json:"field,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

type ValidationReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Valid         bool                `json:"valid"`
	Errors        []ValidationFinding `json:"errors"`
}

// ValidateManifestDetailed performs deterministic, dependency-aware static
// validation. It aggregates independent repairable findings and never executes
// runtime checks.
func ValidateManifestDetailed(raw []byte, projectID string) ValidationReport {
	r := ValidationReport{SchemaVersion: 1, Errors: []ValidationFinding{}}
	var value any
	if err := yaml.Unmarshal(raw, &value); err != nil {
		addRuntimeFinding(&r, "runtime_yaml", "", "", "runtime YAML is invalid")
		return finishRuntimeReport(r)
	}
	collectForbiddenRuntimeFields(value, "", &r)

	top, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(&r, "runtime_schema", "", "", "runtime must be a mapping with schema_version, project_id, environments, relations, and verification")
		return finishRuntimeReport(r)
	}
	reportObjectShape(&r, top, "", "runtime_schema",
		[]string{"schema_version", "project_id", "environments", "relations", "verification"},
		[]string{"schema_version", "project_id", "environments", "relations", "verification"})

	if v, exists := top["schema_version"]; exists {
		if n, ok := v.(int); !ok || n != 1 {
			addRuntimeFinding(&r, "runtime_schema_version", "schema_version", "schema_version", "schema_version must be 1")
		}
	}
	if v, exists := top["project_id"]; exists {
		id, ok := v.(string)
		if !ok || id != projectID {
			addRuntimeFinding(&r, "runtime_project_id", "project_id", "project_id", fmt.Sprintf("project_id must match target %q", projectID))
		}
	}

	declaredIDs := map[string]bool{}
	if v, exists := top["environments"]; exists {
		envs, ok := v.([]any)
		if ok {
			envs = normalizeEnvironmentIDs(envs)
		}
		if !ok || len(envs) < 1 || len(envs) > 64 {
			addRuntimeFinding(&r, "runtime_environments", "environments", "environments", "environments must be a list containing 1 to 64 items")
		} else {
			for i, item := range envs {
				path := fmt.Sprintf("environments[%d]", i)
				id := validateEnvironmentDetailed(item, path, &r)
				if id == "" {
					continue
				}
				if declaredIDs[id] {
					addRuntimeFinding(&r, "runtime_environment_unique", path+".environment_id", "environment_id", fmt.Sprintf("duplicate environment_id %q", id))
				} else {
					declaredIDs[id] = true
				}
			}
		}
	}

	if v, exists := top["relations"]; exists {
		rels, ok := v.([]any)
		if !ok || len(rels) > 128 {
			addRuntimeFinding(&r, "runtime_relations", "relations", "relations", "relations must be a list with at most 128 items")
		} else {
			seen := map[string]bool{}
			for i, item := range rels {
				path := fmt.Sprintf("relations[%d]", i)
				key, complete := validateRelationDetailed(item, path, declaredIDs, &r)
				if complete && seen[key] {
					addRuntimeFinding(&r, "runtime_relation_unique", path, "", "duplicate runtime relation")
				} else if complete {
					seen[key] = true
				}
			}
		}
	}

	if v, exists := top["verification"]; exists {
		validateVerificationDetailed(v, "verification", &r)
	}
	return finishRuntimeReport(r)
}

func validateEnvironmentDetailed(value any, path string, r *ValidationReport) string {
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_environment_schema", path, "", "environment must be a mapping")
		return ""
	}
	required := []string{"environment_id", "kind", "purpose", "description", "workdir", "capabilities", "supervisor", "checks", "boundaries"}
	allowed := append(append([]string{}, required...), "host_id", "repository")
	reportObjectShape(r, m, path, "runtime_environment_schema", required, allowed)

	id := ""
	if v, exists := m["environment_id"]; exists {
		candidate, err := cleanString(v, 64)
		if err != nil || !environmentIDRE.MatchString(candidate) {
			addRuntimeFinding(r, "runtime_environment_id", path+".environment_id", "environment_id", "environment_id must match ^[a-z0-9][a-z0-9._-]{0,63}$")
		} else {
			id = candidate
		}
	}
	if v, exists := m["host_id"]; exists {
		candidate, err := cleanString(v, 64)
		if err != nil || !hostIDRE.MatchString(candidate) {
			addRuntimeFinding(r, "runtime_host_id", path+".host_id", "host_id", "host_id must match ^[a-z0-9][a-z0-9._-]{0,63}$ when provided")
		}
	}
	if v, exists := m["kind"]; exists {
		validateClosedString(r, "runtime_kind", path+".kind", "kind", v, environmentKinds)
	}
	if v, exists := m["purpose"]; exists {
		validateClosedString(r, "runtime_purpose", path+".purpose", "purpose", v, purposes)
	}
	if v, exists := m["description"]; exists {
		if _, err := cleanString(v, 500); err != nil {
			addRuntimeFinding(r, "runtime_description", path+".description", "description", "description must be non-empty single-line text of at most 500 characters")
		}
	}
	if v, exists := m["workdir"]; exists {
		if _, err := absolutePath(v); err != nil {
			addRuntimeFinding(r, "runtime_absolute_path", path+".workdir", "workdir", "workdir must be an absolute path without traversal segments")
		}
	}
	if v, exists := m["repository"]; exists {
		validateRepositoryDetailed(v, path+".repository", r)
	}
	if v, exists := m["capabilities"]; exists {
		validateCapabilitiesDetailed(v, path+".capabilities", r)
	}
	if v, exists := m["supervisor"]; exists {
		validateSupervisorDetailed(v, path+".supervisor", r)
	}
	if v, exists := m["checks"]; exists {
		validateChecksDetailed(v, path+".checks", r)
	}
	if v, exists := m["boundaries"]; exists {
		validateBoundariesDetailed(v, path+".boundaries", r)
	}
	return id
}

func validateRepositoryDetailed(value any, path string, r *ValidationReport) {
	if value == nil {
		return
	}
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_repository", path, "repository", "repository must be null or a mapping with path and branch")
		return
	}
	reportObjectShape(r, m, path, "runtime_repository", []string{"path", "branch"}, []string{"path", "branch", "remote"})
	if v, exists := m["path"]; exists {
		if _, err := absolutePath(v); err != nil {
			addRuntimeFinding(r, "runtime_absolute_path", path+".path", "path", "repository.path must be an absolute path without traversal segments")
		}
	}
	if v, exists := m["branch"]; exists {
		if _, err := cleanString(v, 200); err != nil {
			addRuntimeFinding(r, "runtime_repository", path+".branch", "branch", "repository.branch must be non-empty single-line text of at most 200 characters")
		}
	}
	if v, exists := m["remote"]; exists && v != nil {
		if _, err := cleanString(v, 200); err != nil {
			addRuntimeFinding(r, "runtime_repository", path+".remote", "remote", "repository.remote must be non-empty single-line text of at most 200 characters when provided")
		}
	}
}

func validateCapabilitiesDetailed(value any, path string, r *ValidationReport) {
	xs, ok := value.([]any)
	if !ok || len(xs) > 16 {
		addRuntimeFinding(r, "runtime_capabilities", path, "capabilities", "capabilities must be a list with at most 16 items")
		return
	}
	seen := map[string]bool{}
	for i, raw := range xs {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		s, ok := raw.(string)
		if !ok {
			addRuntimeFinding(r, "runtime_capabilities", itemPath, "capabilities", "capability must be a string; supported values: "+strings.Join(sortedSet(capabilities), ", "))
			continue
		}
		if !capabilities[s] {
			addRuntimeFinding(r, "runtime_capabilities", itemPath, "capabilities", fmt.Sprintf("unsupported capability %q; supported values: %s", s, strings.Join(sortedSet(capabilities), ", ")))
			continue
		}
		if seen[s] {
			addRuntimeFinding(r, "runtime_capabilities", itemPath, "capabilities", fmt.Sprintf("duplicate capability %q", s))
			continue
		}
		seen[s] = true
	}
}

func validateSupervisorDetailed(value any, path string, r *ValidationReport) {
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_supervisor", path, "supervisor", "supervisor must be a mapping")
		return
	}
	rawKind, exists := m["kind"]
	if !exists {
		addRuntimeFinding(r, "runtime_supervisor", path, "kind", "supervisor requires kind")
		return
	}
	kind, ok := rawKind.(string)
	if !ok || !supervisorKinds[kind] {
		validateClosedString(r, "runtime_supervisor", path+".kind", "kind", rawKind, supervisorKinds)
		return // child requirements depend on the discriminator
	}
	var expected []string
	switch kind {
	case "none":
		expected = []string{"kind"}
	case "systemd_user", "systemd_system":
		expected = []string{"kind", "units"}
	case "docker_compose":
		expected = []string{"kind", "compose_file", "services"}
	case "docker":
		expected = []string{"kind", "containers"}
	case "process":
		expected = []string{"kind", "process_name", "pid_file"}
	}
	reportObjectShape(r, m, path, "runtime_supervisor", expected, expected)
	switch kind {
	case "systemd_user", "systemd_system":
		if v, exists := m["units"]; exists {
			validateNameListDetailed(v, path+".units", "units", "runtime_supervisor", 64, r)
		}
	case "docker_compose":
		if v, exists := m["compose_file"]; exists {
			if _, err := absolutePath(v); err != nil {
				addRuntimeFinding(r, "runtime_absolute_path", path+".compose_file", "compose_file", "compose_file must be an absolute path without traversal segments")
			}
		}
		if v, exists := m["services"]; exists {
			validateNameListDetailed(v, path+".services", "services", "runtime_supervisor", 64, r)
		}
	case "docker":
		if v, exists := m["containers"]; exists {
			validateNameListDetailed(v, path+".containers", "containers", "runtime_supervisor", 64, r)
		}
	case "process":
		if v, exists := m["process_name"]; exists {
			if _, err := cleanString(v, 200); err != nil {
				addRuntimeFinding(r, "runtime_supervisor", path+".process_name", "process_name", "process_name must be non-empty single-line text of at most 200 characters")
			}
		}
		if v, exists := m["pid_file"]; exists {
			if _, err := absolutePath(v); err != nil {
				addRuntimeFinding(r, "runtime_absolute_path", path+".pid_file", "pid_file", "pid_file must be an absolute path without traversal segments")
			}
		}
	}
}

func validateChecksDetailed(value any, path string, r *ValidationReport) {
	xs, ok := value.([]any)
	if !ok || len(xs) > 32 {
		addRuntimeFinding(r, "runtime_checks", path, "checks", "checks must be a list with at most 32 items")
		return
	}
	for i, raw := range xs {
		validateCheckDetailed(raw, fmt.Sprintf("%s[%d]", path, i), r)
	}
}

func validateCheckDetailed(value any, path string, r *ValidationReport) {
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_check", path, "", "check must be a mapping")
		return
	}
	rawKind, exists := m["kind"]
	if !exists {
		addRuntimeFinding(r, "runtime_check", path, "kind", "check requires kind")
		return
	}
	kind, ok := rawKind.(string)
	if !ok || !checkKinds[kind] {
		validateClosedString(r, "runtime_check", path+".kind", "kind", rawKind, checkKinds)
		return // child requirements depend on the discriminator
	}
	switch kind {
	case "git_status", "git_head":
		reportObjectShape(r, m, path, "runtime_check", []string{"kind"}, []string{"kind", "target"})
		if target, exists := m["target"]; exists && target != "repository" {
			addRuntimeFinding(r, "runtime_check", path+".target", "target", fmt.Sprintf("%s target must be %q", kind, "repository"))
		}
	case "systemd_units":
		reportObjectShape(r, m, path, "runtime_check", []string{"kind", "scope", "units"}, []string{"kind", "scope", "units"})
		if v, exists := m["scope"]; exists {
			validateTwoValueString(r, "runtime_check", path+".scope", "scope", v, "user", "system")
		}
		if v, exists := m["units"]; exists {
			validateNameListDetailed(v, path+".units", "units", "runtime_check", 64, r)
		}
	case "failed_units":
		reportObjectShape(r, m, path, "runtime_check", []string{"kind", "scope"}, []string{"kind", "scope"})
		if v, exists := m["scope"]; exists {
			validateTwoValueString(r, "runtime_check", path+".scope", "scope", v, "user", "system")
		}
	case "docker_compose_services":
		reportObjectShape(r, m, path, "runtime_check", []string{"kind", "compose_file", "services"}, []string{"kind", "compose_file", "services"})
		if v, exists := m["compose_file"]; exists {
			if _, err := absolutePath(v); err != nil {
				addRuntimeFinding(r, "runtime_absolute_path", path+".compose_file", "compose_file", "compose_file must be an absolute path without traversal segments")
			}
		}
		if v, exists := m["services"]; exists {
			validateNameListDetailed(v, path+".services", "services", "runtime_check", 64, r)
		}
	case "file_exists", "directory_exists":
		reportObjectShape(r, m, path, "runtime_check", []string{"kind", "path"}, []string{"kind", "path"})
		if v, exists := m["path"]; exists {
			if _, err := absolutePath(v); err != nil {
				addRuntimeFinding(r, "runtime_absolute_path", path+".path", "path", "check path must be an absolute path without traversal segments")
			}
		}
	}
}

func validateBoundariesDetailed(value any, path string, r *ValidationReport) {
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_boundaries", path, "boundaries", "boundaries must be a mapping")
		return
	}
	reportObjectShape(r, m, path, "runtime_boundaries",
		[]string{"production_writer", "mutation_requires_explicit_authorization"},
		[]string{"production_writer", "single_writer", "mutation_requires_explicit_authorization"})
	for _, key := range []string{"production_writer", "single_writer", "mutation_requires_explicit_authorization"} {
		if v, exists := m[key]; exists {
			if _, ok := v.(bool); !ok {
				addRuntimeFinding(r, "runtime_boundaries", path+"."+key, key, "boundary value must be boolean")
			}
		}
	}
}

func validateRelationDetailed(value any, path string, ids map[string]bool, r *ValidationReport) (string, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_relation", path, "", "relation must be a mapping with kind, from, and to")
		return "", false
	}
	reportObjectShape(r, m, path, "runtime_relation", []string{"kind", "from", "to"}, []string{"kind", "from", "to"})
	kind, kindOK := m["kind"].(string)
	if raw, exists := m["kind"]; exists {
		if !kindOK || !relationKinds[kind] {
			validateClosedString(r, "runtime_relation", path+".kind", "kind", raw, relationKinds)
			kindOK = false
		}
	} else {
		kindOK = false
	}
	from, fromOK := m["from"].(string)
	to, toOK := m["to"].(string)
	if raw, exists := m["from"]; exists && (!fromOK || strings.TrimSpace(from) == "" || !ids[from]) {
		actual := fmt.Sprintf("%v", raw)
		if fromOK {
			actual = fmt.Sprintf("%q", from)
		}
		addRuntimeFinding(r, "runtime_relation_reference", path+".from", "from", "relation from must reference a declared environment_id; received "+actual)
		fromOK = false
	}
	if raw, exists := m["to"]; exists && (!toOK || strings.TrimSpace(to) == "" || !ids[to]) {
		actual := fmt.Sprintf("%v", raw)
		if toOK {
			actual = fmt.Sprintf("%q", to)
		}
		addRuntimeFinding(r, "runtime_relation_reference", path+".to", "to", "relation to must reference a declared environment_id; received "+actual)
		toOK = false
	}
	if fromOK && toOK && from == to {
		addRuntimeFinding(r, "runtime_relation_reference", path, "", "relation from and to must reference different environments")
		return "", false
	}
	if !kindOK || !fromOK || !toOK {
		return "", false
	}
	return kind + "\x00" + from + "\x00" + to, true
}

func validateVerificationDetailed(value any, path string, r *ValidationReport) {
	if value == nil {
		return
	}
	m, ok := value.(map[string]any)
	if !ok {
		addRuntimeFinding(r, "runtime_verification", path, "verification", "verification must be empty or a complete mapping")
		return
	}
	if len(m) == 0 {
		return
	}
	required := []string{"verified_at", "verified_by", "stale_after", "evidence"}
	reportObjectShape(r, m, path, "runtime_verification", required, required)
	if v, exists := m["verified_at"]; exists {
		s, err := cleanString(v, 100)
		if err != nil {
			addRuntimeFinding(r, "runtime_verification", path+".verified_at", "verified_at", "verified_at must be an RFC3339 timestamp")
		} else if _, err := time.Parse(time.RFC3339, s); err != nil {
			addRuntimeFinding(r, "runtime_verification", path+".verified_at", "verified_at", fmt.Sprintf("verified_at %q is not a valid RFC3339 timestamp", s))
		}
	}
	if v, exists := m["verified_by"]; exists {
		if _, err := cleanString(v, 200); err != nil {
			addRuntimeFinding(r, "runtime_verification", path+".verified_by", "verified_by", "verified_by must be non-empty single-line text of at most 200 characters")
		}
	}
	if v, exists := m["stale_after"]; exists {
		s, err := cleanString(v, 32)
		if err != nil || !durationRE.MatchString(s) {
			addRuntimeFinding(r, "runtime_verification", path+".stale_after", "stale_after", "stale_after must use PnD duration with a positive day count")
		}
	}
	if v, exists := m["evidence"]; exists {
		xs, ok := v.([]any)
		if !ok || len(xs) < 1 || len(xs) > 32 {
			addRuntimeFinding(r, "runtime_verification", path+".evidence", "evidence", "verification evidence must contain 1 to 32 memory:// Authority URIs")
			return
		}
		for i, raw := range xs {
			s, err := cleanString(raw, 500)
			if err != nil || !strings.HasPrefix(s, "memory://") {
				addRuntimeFinding(r, "runtime_verification", fmt.Sprintf("%s.evidence[%d]", path, i), "evidence", "verification evidence must use a memory:// Authority URI")
			}
		}
	}
}

func validateClosedString(r *ValidationReport, code, path, field string, raw any, allowed map[string]bool) bool {
	values := strings.Join(sortedSet(allowed), ", ")
	s, ok := raw.(string)
	if !ok {
		addRuntimeFinding(r, code, path, field, fmt.Sprintf("%s must be a string; supported values: %s", field, values))
		return false
	}
	if !allowed[s] {
		addRuntimeFinding(r, code, path, field, fmt.Sprintf("unsupported %s %q; supported values: %s", field, s, values))
		return false
	}
	return true
}

func validateTwoValueString(r *ValidationReport, code, path, field string, raw any, a, b string) bool {
	s, ok := raw.(string)
	if !ok || (s != a && s != b) {
		if ok {
			addRuntimeFinding(r, code, path, field, fmt.Sprintf("unsupported %s %q; supported values: %s, %s", field, s, a, b))
		} else {
			addRuntimeFinding(r, code, path, field, fmt.Sprintf("%s must be a string; supported values: %s, %s", field, a, b))
		}
		return false
	}
	return true
}

func validateNameListDetailed(value any, path, field, code string, max int, r *ValidationReport) {
	xs, ok := value.([]any)
	if !ok || len(xs) == 0 || len(xs) > max {
		addRuntimeFinding(r, code, path, field, fmt.Sprintf("%s must be a non-empty list with at most %d items", field, max))
		return
	}
	seen := map[string]bool{}
	for i, raw := range xs {
		s, err := cleanString(raw, 200)
		if err != nil {
			addRuntimeFinding(r, code, fmt.Sprintf("%s[%d]", path, i), field, fmt.Sprintf("%s item must be non-empty single-line text of at most 200 characters", field))
			continue
		}
		if seen[s] {
			addRuntimeFinding(r, code, fmt.Sprintf("%s[%d]", path, i), field, fmt.Sprintf("duplicate %s item %q", field, s))
			continue
		}
		seen[s] = true
	}
}

func reportObjectShape(r *ValidationReport, m map[string]any, path, code string, required, allowed []string) {
	missing := make([]string, 0)
	extra := make([]string, 0)
	allow := map[string]bool{}
	for _, key := range allowed {
		allow[key] = true
	}
	for _, key := range required {
		if _, exists := m[key]; !exists {
			missing = append(missing, key)
		}
	}
	for key := range m {
		if !allow[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) == 0 && len(extra) == 0 {
		return
	}
	parts := make([]string, 0, 2)
	if len(missing) > 0 {
		parts = append(parts, "missing required fields: "+strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		parts = append(parts, "unsupported fields: "+strings.Join(extra, ", "))
	}
	addRuntimeFinding(r, code, path, "", strings.Join(parts, "; "))
}

func collectForbiddenRuntimeFields(value any, path string, r *ValidationReport) {
	switch x := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if forbiddenKeyRE.MatchString(key) {
				addRuntimeFinding(r, "runtime_forbidden_field", childPath, key, "forbidden runtime field "+key)
			}
			collectForbiddenRuntimeFields(x[key], childPath, r)
		}
	case []any:
		for i, item := range x {
			childPath := fmt.Sprintf("[%d]", i)
			if path != "" {
				childPath = fmt.Sprintf("%s[%d]", path, i)
			}
			collectForbiddenRuntimeFields(item, childPath, r)
		}
	}
}

func addRuntimeFinding(r *ValidationReport, code, path, field, message string) {
	r.Errors = append(r.Errors, ValidationFinding{Code: code, Path: path, Field: field, Message: message})
}

func finishRuntimeReport(r ValidationReport) ValidationReport {
	sort.SliceStable(r.Errors, func(i, j int) bool {
		a, b := r.Errors[i], r.Errors[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Message < b.Message
	})
	r.Valid = len(r.Errors) == 0
	return r
}

func sortedSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
