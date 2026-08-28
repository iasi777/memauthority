// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package runtimeview

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/iasi777/v-memory/internal/gitview"
	"gopkg.in/yaml.v3"
)

var (
	environmentIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	durationRE      = regexp.MustCompile(`^P([1-9][0-9]*)D$`)
	forbiddenKeyRE  = regexp.MustCompile(`(?i)(token|password|secret|api[_-]?key|credential|command|shell|private[_-]?key)`)
)

var hostIDs = stringSet("local", "arm", "amd")
var environmentKinds = stringSet("workspace", "deployment", "service", "archive")
var purposes = stringSet("source_development", "test", "staging", "production", "production_reader", "disabled_mirror", "backup")
var capabilities = stringSet("edit", "test", "build", "runtime_check", "deploy_source", "deploy_target", "read_only")
var supervisorKinds = stringSet("none", "systemd_user", "systemd_system", "docker_compose", "docker", "process")
var relationKinds = stringSet("deploys_to", "mirrors")
var checkKinds = stringSet("git_status", "git_head", "systemd_units", "failed_units", "docker_compose_services", "file_exists", "directory_exists")

type Registration struct {
	ProjectID       string
	Lifecycle       string
	RuntimeResource string
	ArchivedAt      string
	ArchiveReason   string
}

type Service struct {
	root         string
	sourceCommit string
	projects     map[string]Registration
	now          func() time.Time
}

type QueryArgs struct {
	ProjectID                string `json:"project_id"`
	HostID                   string `json:"host_id,omitempty"`
	EnvironmentID            string `json:"environment_id,omitempty"`
	Detail                   string `json:"detail,omitempty"`
	ExpectedResourceRevision string `json:"expected_resource_revision,omitempty"`
}

func New(root, sourceCommit string, projects map[string]Registration) *Service {
	copied := make(map[string]Registration, len(projects))
	for k, v := range projects {
		copied[k] = v
	}
	return &Service{root: root, sourceCommit: sourceCommit, projects: copied, now: time.Now}
}

func (s *Service) ProjectRuntime(args QueryArgs) map[string]any {
	out := emptyOutput()
	out["project_id"] = args.ProjectID
	out["resource_uri"] = "memory://projects/" + args.ProjectID + "/runtime"

	reg, ok := s.projects[args.ProjectID]
	if !ok {
		out["status"] = "error"
		out["code"] = "runtime_failed"
		out["message"] = "unknown project"
		return out
	}
	if reg.Lifecycle == "archived" {
		out["status"] = "archived"
		if reg.ArchivedAt != "" {
			out["archived_at"] = reg.ArchivedAt
		}
		if reg.ArchiveReason != "" {
			out["archive_reason"] = reg.ArchiveReason
		}
		return out
	}
	if reg.RuntimeResource == "" {
		out["status"] = "runtime_missing"
		return out
	}
	detail := args.Detail
	if detail == "" {
		detail = "checks"
	}
	if detail != "summary" && detail != "checks" && detail != "full" {
		out["status"] = "error"
		out["code"] = "runtime_failed"
		out["message"] = "invalid runtime detail"
		return out
	}
	if args.HostID != "" && !hostIDs[args.HostID] {
		out["status"] = "error"
		out["code"] = "runtime_failed"
		out["message"] = "invalid host_id"
		return out
	}

	raw, err := s.readRuntime(reg)
	if err != nil {
		out["status"] = "error"
		out["code"] = "runtime_failed"
		out["message"] = err.Error()
		return out
	}
	manifest, err := parseManifest(raw, args.ProjectID)
	if err != nil {
		out["status"] = "error"
		out["code"] = "runtime_failed"
		out["message"] = err.Error()
		return out
	}
	sum := sha256.Sum256(raw)
	revision := "sha256:" + hex.EncodeToString(sum[:])
	if args.ExpectedResourceRevision != "" && args.ExpectedResourceRevision != revision {
		out["status"] = "error"
		out["code"] = "conflict"
		out["message"] = "runtime revision does not match current content"
		out["current_revision"] = revision
		return out
	}
	stale, err := staleness(manifest, s.now())
	if err != nil {
		out["status"] = "error"
		out["code"] = "runtime_failed"
		out["message"] = err.Error()
		return out
	}
	view := renderView(manifest, args.HostID, args.EnvironmentID, detail, stale)
	for k, v := range view {
		out[k] = v
	}
	out["project_id"] = args.ProjectID
	out["resource_revision"] = revision
	out["content_hash"] = revision
	out["source_commit"] = s.sourceCommit
	out["staleness"] = stale
	return out
}

// ReadRawResource returns the validated raw runtime manifest for the distinct
// runtime resource URI. It never executes declared checks.
func (s *Service) ReadRawResource(projectID string) (string, string, error) {
	reg, ok := s.projects[projectID]
	if !ok || reg.Lifecycle != "active" || reg.RuntimeResource == "" {
		return "", "", fmt.Errorf("runtime resource not found")
	}
	raw, err := s.readRuntime(reg)
	if err != nil {
		return "", "", err
	}
	if _, err := parseManifest(raw, projectID); err != nil {
		return "", "", err
	}
	return "memory://projects/" + projectID + "/runtime", string(raw), nil
}

func emptyOutput() map[string]any {
	return map[string]any{
		"status": nil, "code": nil, "message": nil, "rule": nil, "current_revision": nil,
		"project_id": nil, "resource_uri": nil, "resource_revision": nil, "content_hash": nil,
		"source_commit": nil, "staleness": nil, "available_hosts": nil, "environments": nil,
		"relations": nil, "verification": nil, "archived_at": nil, "archive_reason": nil,
	}
}

func (s *Service) readRuntime(reg Registration) ([]byte, error) {
	if reg.RuntimeResource != "projects/"+reg.ProjectID+"/runtime.yaml" {
		return nil, fmt.Errorf("runtime resource path is invalid")
	}
	if strings.TrimSpace(s.sourceCommit) == "" {
		return nil, fmt.Errorf("runtime owned commit is unavailable")
	}
	raw, err := gitview.ReadFile(s.root, s.sourceCommit, reg.RuntimeResource)
	if err != nil {
		return nil, fmt.Errorf("runtime resource does not exist at owned commit")
	}
	if len(raw) > 2*1024*1024 {
		return nil, fmt.Errorf("runtime exceeds source limit")
	}
	return raw, nil
}

type manifest struct {
	Environments []map[string]any
	Relations    []map[string]any
	Verification map[string]any
}

// ValidateManifest preserves the fail-fast compatibility entrypoint for callers
// that do not consume the detailed report. Candidate writes should prefer
// ValidateManifestDetailed so independent repairable issues can be returned at once.
func ValidateManifest(raw []byte, projectID string) error {
	report := ValidateManifestDetailed(raw, projectID)
	if report.Valid {
		return nil
	}
	if len(report.Errors) == 0 {
		return fmt.Errorf("runtime: runtime validation failed")
	}
	finding := report.Errors[0]
	return fmt.Errorf("%s: %s", finding.Code, finding.Message)
}

func parseManifest(raw []byte, projectID string) (manifest, error) {
	var value any
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return manifest{}, fmt.Errorf("runtime_yaml: runtime YAML is invalid")
	}
	if err := rejectForbidden(value, "runtime"); err != nil {
		return manifest{}, err
	}
	top, ok := value.(map[string]any)
	if !ok || !exactKeys(top, []string{"schema_version", "project_id", "environments", "relations", "verification"}) {
		return manifest{}, fmt.Errorf("runtime_schema: runtime requires schema_version, project_id, environments, relations, verification")
	}
	if top["schema_version"] != 1 {
		return manifest{}, fmt.Errorf("runtime_schema_version: runtime schema_version must be 1")
	}
	if top["project_id"] != projectID {
		return manifest{}, fmt.Errorf("runtime_project_id: runtime project_id does not match target")
	}
	envRaw, ok := top["environments"].([]any)
	if !ok || len(envRaw) < 1 || len(envRaw) > 64 {
		return manifest{}, fmt.Errorf("runtime_environments: runtime environments must contain 1 to 64 items")
	}
	envs := make([]map[string]any, 0, len(envRaw))
	ids := map[string]bool{}
	for _, item := range envRaw {
		env, err := validateEnvironment(item)
		if err != nil {
			return manifest{}, err
		}
		id := env["environment_id"].(string)
		if ids[id] {
			return manifest{}, fmt.Errorf("runtime_environment_unique: duplicate environment_id: %s", id)
		}
		ids[id] = true
		envs = append(envs, env)
	}
	relRaw, ok := top["relations"].([]any)
	if !ok || len(relRaw) > 128 {
		return manifest{}, fmt.Errorf("runtime_relations: runtime relations must be a list")
	}
	rels := make([]map[string]any, 0, len(relRaw))
	seen := map[string]bool{}
	for _, item := range relRaw {
		rel, err := validateRelation(item, ids)
		if err != nil {
			return manifest{}, err
		}
		key := rel["kind"].(string) + "\x00" + rel["from"].(string) + "\x00" + rel["to"].(string)
		if seen[key] {
			return manifest{}, fmt.Errorf("runtime_relation_unique: duplicate runtime relation")
		}
		seen[key] = true
		rels = append(rels, rel)
	}
	verification, err := validateVerification(top["verification"])
	if err != nil {
		return manifest{}, err
	}
	return manifest{Environments: envs, Relations: rels, Verification: verification}, nil
}

func validateEnvironment(value any) (map[string]any, error) {
	m, ok := value.(map[string]any)
	required := []string{"environment_id", "host_id", "kind", "purpose", "description", "workdir", "capabilities", "supervisor", "checks", "boundaries"}
	if !ok || !requiredSubsetAllowed(m, required, append(required, "repository")) {
		return nil, fmt.Errorf("runtime_environment_schema: invalid environment fields")
	}
	id, err := cleanString(m["environment_id"], 64)
	if err != nil || !environmentIDRE.MatchString(id) {
		return nil, fmt.Errorf("runtime_environment_id: environment_id is invalid")
	}
	host, ok := m["host_id"].(string)
	if !ok || !hostIDs[host] {
		return nil, fmt.Errorf("runtime_host_id: unsupported host_id")
	}
	kind, ok := m["kind"].(string)
	if !ok || !environmentKinds[kind] {
		return nil, fmt.Errorf("runtime_kind: unsupported environment kind")
	}
	purpose, ok := m["purpose"].(string)
	if !ok || !purposes[purpose] {
		return nil, fmt.Errorf("runtime_purpose: unsupported purpose")
	}
	description, err := cleanString(m["description"], 500)
	if err != nil {
		return nil, fmt.Errorf("runtime_description: description is invalid")
	}
	workdir, err := absolutePath(m["workdir"])
	if err != nil {
		return nil, fmt.Errorf("runtime_absolute_path: workdir must be an absolute path")
	}
	repository, err := validateRepository(m["repository"])
	if err != nil {
		return nil, err
	}
	caps, err := enumList(m["capabilities"], capabilities, 16, true)
	if err != nil {
		return nil, fmt.Errorf("runtime_capabilities: unsupported capabilities value")
	}
	supervisor, err := validateSupervisor(m["supervisor"])
	if err != nil {
		return nil, err
	}
	checkRaw, ok := m["checks"].([]any)
	if !ok || len(checkRaw) > 32 {
		return nil, fmt.Errorf("runtime_checks: checks must be a list with at most 32 items")
	}
	checks := make([]map[string]any, 0, len(checkRaw))
	for _, c := range checkRaw {
		x, e := validateCheck(c)
		if e != nil {
			return nil, e
		}
		checks = append(checks, x)
	}
	boundaries, err := validateBoundaries(m["boundaries"])
	if err != nil {
		return nil, err
	}
	return map[string]any{"environment_id": id, "host_id": host, "kind": kind, "purpose": purpose, "description": description, "workdir": workdir, "repository": repository, "capabilities": caps, "supervisor": supervisor, "checks": checks, "boundaries": boundaries}, nil
}

func validateRepository(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	m, ok := value.(map[string]any)
	if !ok || !requiredSubsetAllowed(m, []string{"path", "branch"}, []string{"path", "branch", "remote"}) {
		return nil, fmt.Errorf("runtime_repository: repository requires path and branch")
	}
	path, err := absolutePath(m["path"])
	if err != nil {
		return nil, fmt.Errorf("runtime_absolute_path: repository.path must be an absolute path")
	}
	branch, err := cleanString(m["branch"], 200)
	if err != nil {
		return nil, fmt.Errorf("runtime_repository: repository branch is invalid")
	}
	out := map[string]any{"path": path, "branch": branch}
	if v, exists := m["remote"]; exists && v != nil {
		remote, e := cleanString(v, 200)
		if e != nil {
			return nil, fmt.Errorf("runtime_repository: repository remote is invalid")
		}
		out["remote"] = remote
	}
	return out, nil
}

func validateSupervisor(value any) (map[string]any, error) {
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("runtime_supervisor: unsupported supervisor")
	}
	kind, ok := m["kind"].(string)
	if !ok || !supervisorKinds[kind] {
		return nil, fmt.Errorf("runtime_supervisor: unsupported supervisor")
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
	if !exactKeys(m, expected) {
		return nil, fmt.Errorf("runtime_supervisor: invalid fields for supervisor %s", kind)
	}
	out := map[string]any{"kind": kind}
	if v, ok := m["units"]; ok {
		xs, e := nameList(v, 64)
		if e != nil {
			return nil, e
		}
		out["units"] = xs
	}
	if v, ok := m["compose_file"]; ok {
		p, e := absolutePath(v)
		if e != nil {
			return nil, e
		}
		out["compose_file"] = p
		xs, e := nameList(m["services"], 64)
		if e != nil {
			return nil, e
		}
		out["services"] = xs
	}
	if v, ok := m["containers"]; ok {
		xs, e := nameList(v, 64)
		if e != nil {
			return nil, e
		}
		out["containers"] = xs
	}
	if kind == "process" {
		n, e := cleanString(m["process_name"], 200)
		if e != nil {
			return nil, e
		}
		p, e := absolutePath(m["pid_file"])
		if e != nil {
			return nil, e
		}
		out["process_name"] = n
		out["pid_file"] = p
	}
	return out, nil
}

func validateCheck(value any) (map[string]any, error) {
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("runtime_check: unsupported check kind")
	}
	kind, ok := m["kind"].(string)
	if !ok || !checkKinds[kind] {
		return nil, fmt.Errorf("runtime_check: unsupported check kind")
	}
	switch kind {
	case "git_status", "git_head":
		if !requiredSubsetAllowed(m, []string{"kind"}, []string{"kind", "target"}) {
			return nil, fmt.Errorf("runtime_check: invalid %s check", kind)
		}
		if target, exists := m["target"]; exists && target != "repository" {
			return nil, fmt.Errorf("runtime_check: %s must target repository", kind)
		}
		return map[string]any{"kind": kind, "target": "repository"}, nil
	case "systemd_units":
		if !exactKeys(m, []string{"kind", "scope", "units"}) || (m["scope"] != "user" && m["scope"] != "system") {
			return nil, fmt.Errorf("runtime_check: invalid systemd_units check")
		}
		xs, e := nameList(m["units"], 64)
		if e != nil {
			return nil, e
		}
		return map[string]any{"kind": kind, "scope": m["scope"], "units": xs}, nil
	case "failed_units":
		if !exactKeys(m, []string{"kind", "scope"}) || (m["scope"] != "user" && m["scope"] != "system") {
			return nil, fmt.Errorf("runtime_check: invalid failed_units check")
		}
		return map[string]any{"kind": kind, "scope": m["scope"]}, nil
	case "docker_compose_services":
		if !exactKeys(m, []string{"kind", "compose_file", "services"}) {
			return nil, fmt.Errorf("runtime_check: invalid docker_compose_services check")
		}
		p, e := absolutePath(m["compose_file"])
		if e != nil {
			return nil, e
		}
		xs, e := nameList(m["services"], 64)
		if e != nil {
			return nil, e
		}
		return map[string]any{"kind": kind, "compose_file": p, "services": xs}, nil
	default:
		if !exactKeys(m, []string{"kind", "path"}) {
			return nil, fmt.Errorf("runtime_check: invalid %s check", kind)
		}
		p, e := absolutePath(m["path"])
		if e != nil {
			return nil, e
		}
		return map[string]any{"kind": kind, "path": p}, nil
	}
}

func validateBoundaries(value any) (map[string]any, error) {
	m, ok := value.(map[string]any)
	if !ok || !requiredSubsetAllowed(m, []string{"production_writer", "mutation_requires_explicit_authorization"}, []string{"production_writer", "single_writer", "mutation_requires_explicit_authorization"}) {
		return nil, fmt.Errorf("runtime_boundaries: invalid boundaries fields")
	}
	out := map[string]any{}
	for _, k := range []string{"production_writer", "mutation_requires_explicit_authorization", "single_writer"} {
		if v, exists := m[k]; exists {
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("runtime_boundaries: boundary values must be booleans")
			}
			out[k] = b
		}
	}
	return out, nil
}

func validateRelation(value any, ids map[string]bool) (map[string]any, error) {
	m, ok := value.(map[string]any)
	if !ok || !exactKeys(m, []string{"kind", "from", "to"}) {
		return nil, fmt.Errorf("runtime_relation: relation requires kind, from, and to")
	}
	kind, kok := m["kind"].(string)
	from, fok := m["from"].(string)
	to, tok := m["to"].(string)
	if !kok || !relationKinds[kind] {
		return nil, fmt.Errorf("runtime_relation: unsupported relation kind")
	}
	if !fok || !tok || !ids[from] || !ids[to] || from == to {
		return nil, fmt.Errorf("runtime_relation_reference: relation references are invalid")
	}
	return map[string]any{"kind": kind, "from": from, "to": to}, nil
}

func validateVerification(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("runtime_verification: verification must be empty or complete")
	}
	if len(m) == 0 {
		return map[string]any{}, nil
	}
	if !exactKeys(m, []string{"verified_at", "verified_by", "stale_after", "evidence"}) {
		return nil, fmt.Errorf("runtime_verification: verification must be empty or complete")
	}
	at, e := cleanString(m["verified_at"], 100)
	if e != nil {
		return nil, e
	}
	if _, e = time.Parse(time.RFC3339, at); e != nil {
		return nil, fmt.Errorf("runtime_verification: verified_at must be an ISO datetime")
	}
	by, e := cleanString(m["verified_by"], 200)
	if e != nil {
		return nil, e
	}
	dur, e := cleanString(m["stale_after"], 32)
	if e != nil || !durationRE.MatchString(dur) {
		return nil, fmt.Errorf("runtime_verification: stale_after must use PnD duration")
	}
	evRaw, ok := m["evidence"].([]any)
	if !ok || len(evRaw) < 1 || len(evRaw) > 32 {
		return nil, fmt.Errorf("runtime_verification: verification evidence is required")
	}
	ev := make([]string, 0, len(evRaw))
	for _, x := range evRaw {
		s, e := cleanString(x, 500)
		if e != nil || !strings.HasPrefix(s, "memory://") {
			return nil, fmt.Errorf("runtime_verification: verification evidence must use memory:// URIs; write observed facts to a memory role such as handoff or progress, then reference that Authority resource here")
		}
		ev = append(ev, s)
	}
	return map[string]any{"verified_at": at, "verified_by": by, "stale_after": dur, "evidence": ev}, nil
}

func staleness(m manifest, now time.Time) (string, error) {
	v := m.Verification
	raw, ok := v["verified_at"]
	if !ok {
		return "unverified", nil
	}
	at, err := time.Parse(time.RFC3339, raw.(string))
	if err != nil {
		return "", err
	}
	match := durationRE.FindStringSubmatch(v["stale_after"].(string))
	if match == nil {
		return "", fmt.Errorf("invalid stale_after")
	}
	days := 0
	for _, c := range match[1] {
		days = days*10 + int(c-'0')
	}
	if now.After(at.Add(time.Duration(days) * 24 * time.Hour)) {
		return "stale", nil
	}
	return "current", nil
}

func renderView(m manifest, hostID, environmentID, detail, stale string) map[string]any {
	hostsSet := map[string]bool{}
	for _, e := range m.Environments {
		hostsSet[e["host_id"].(string)] = true
	}
	hosts := make([]string, 0, len(hostsSet))
	for h := range hostsSet {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	verification := copyMap(m.Verification)
	delete(verification, "evidence")
	if detail == "full" {
		if e, ok := m.Verification["evidence"]; ok {
			verification["evidence"] = e
		}
	}
	verification["staleness"] = stale
	if hostID == "" && environmentID == "" {
		envs := make([]map[string]any, 0, len(m.Environments))
		for _, e := range m.Environments {
			envs = append(envs, environmentSummary(e))
		}
		return map[string]any{"status": "ok", "available_hosts": hosts, "environments": envs, "relations": nil, "verification": verification}
	}
	if hostID != "" && !hostsSet[hostID] {
		return map[string]any{"status": "host_not_registered", "available_hosts": hosts, "environments": []map[string]any{}, "relations": nil, "verification": verification}
	}
	selected := make([]map[string]any, 0)
	for _, e := range m.Environments {
		if hostID != "" && e["host_id"] != hostID {
			continue
		}
		if environmentID != "" && e["environment_id"] != environmentID {
			continue
		}
		selected = append(selected, e)
	}
	if environmentID != "" && len(selected) == 0 {
		return map[string]any{"status": "environment_not_registered", "message": "environment_id is not registered for the requested runtime scope", "available_hosts": hosts, "environments": []map[string]any{}, "relations": nil, "verification": verification}
	}
	rendered := make([]map[string]any, 0, len(selected))
	ids := map[string]bool{}
	for _, e := range selected {
		rendered = append(rendered, environmentDetail(e, detail))
		ids[e["environment_id"].(string)] = true
	}
	var relations any = nil
	if detail == "full" {
		r := make([]map[string]any, 0)
		for _, rel := range m.Relations {
			if ids[rel["from"].(string)] || ids[rel["to"].(string)] {
				r = append(r, copyMap(rel))
			}
		}
		relations = r
	}
	return map[string]any{"status": "ok", "available_hosts": hosts, "environments": rendered, "relations": relations, "verification": verification}
}

func environmentSummary(e map[string]any) map[string]any {
	keys := []string{"environment_id", "host_id", "kind", "purpose", "description", "workdir", "repository", "capabilities"}
	out := map[string]any{}
	for _, k := range keys {
		out[k] = e[k]
	}
	return out
}
func environmentDetail(e map[string]any, detail string) map[string]any {
	out := environmentSummary(e)
	if detail == "checks" || detail == "full" {
		out["supervisor"] = e["supervisor"]
		out["checks"] = e["checks"]
	}
	if detail == "full" {
		out["boundaries"] = e["boundaries"]
	}
	return out
}

func rejectForbidden(v any, path string) error {
	switch x := v.(type) {
	case map[string]any:
		for k, item := range x {
			if forbiddenKeyRE.MatchString(k) {
				return fmt.Errorf("runtime_forbidden_field: forbidden runtime field at %s.%s", path, k)
			}
			if err := rejectForbidden(item, path+"."+k); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range x {
			if err := rejectForbidden(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
func cleanString(v any, max int) (string, error) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" || strings.ContainsAny(s, "\r\n") {
		return "", fmt.Errorf("invalid text")
	}
	s = strings.TrimSpace(s)
	if len([]rune(s)) > max {
		return "", fmt.Errorf("text too long")
	}
	return s, nil
}
func absolutePath(v any) (string, error) {
	s, e := cleanString(v, 500)
	if e != nil {
		return "", e
	}
	if !strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("path must be absolute")
	}
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "." || p == ".." {
			return "", fmt.Errorf("path contains traversal")
		}
	}
	return s, nil
}
func enumList(v any, allowed map[string]bool, max int, allowEmpty bool) ([]string, error) {
	xs, ok := v.([]any)
	if !ok || len(xs) > max || (!allowEmpty && len(xs) == 0) {
		return nil, fmt.Errorf("invalid list")
	}
	out := make([]string, 0, len(xs))
	seen := map[string]bool{}
	for _, x := range xs {
		s, ok := x.(string)
		if !ok || !allowed[s] || seen[s] {
			return nil, fmt.Errorf("invalid enum")
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}
func nameList(v any, max int) ([]string, error) {
	xs, ok := v.([]any)
	if !ok || len(xs) == 0 || len(xs) > max {
		return nil, fmt.Errorf("invalid name list")
	}
	out := make([]string, 0, len(xs))
	seen := map[string]bool{}
	for _, x := range xs {
		s, e := cleanString(x, 200)
		if e != nil || seen[s] {
			return nil, fmt.Errorf("invalid name list")
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}
func exactKeys(m map[string]any, keys []string) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
func requiredSubsetAllowed(m map[string]any, required, allowed []string) bool {
	a := map[string]bool{}
	for _, k := range allowed {
		a[k] = true
	}
	for k := range m {
		if !a[k] {
			return false
		}
	}
	for _, k := range required {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
func stringSet(values ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}
