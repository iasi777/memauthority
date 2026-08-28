// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package runtimeview

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestProjectRuntimeViews(t *testing.T) {
	fixture, head := runtimeFixture(t)
	svc := New(fixture, head, map[string]Registration{
		"demo":  {ProjectID: "demo", Lifecycle: "active", RuntimeResource: "projects/demo/runtime.yaml"},
		"other": {ProjectID: "other", Lifecycle: "active"},
	})
	summary := svc.ProjectRuntime(QueryArgs{ProjectID: "demo", Detail: "summary"})
	if summary["status"] != "ok" || summary["source_commit"] != head {
		t.Fatalf("summary: %#v", summary)
	}
	envs := summary["environments"].([]map[string]any)
	if len(envs) != 2 || envs[0]["environment_id"] != "local-workspace" || envs[1]["environment_id"] != "arm-production" {
		t.Fatalf("summary environments: %#v", envs)
	}
	if _, ok := envs[0]["checks"]; ok {
		t.Fatal("summary unexpectedly exposed checks")
	}

	full := svc.ProjectRuntime(QueryArgs{ProjectID: "demo", EnvironmentID: "arm-production", Detail: "full"})
	if full["status"] != "ok" {
		t.Fatalf("full: %#v", full)
	}
	selected := full["environments"].([]map[string]any)[0]
	if selected["boundaries"].(map[string]any)["production_writer"] != true {
		t.Fatalf("full boundaries: %#v", selected)
	}
	if len(full["relations"].([]map[string]any)) != 1 {
		t.Fatalf("full relations: %#v", full["relations"])
	}

	missing := svc.ProjectRuntime(QueryArgs{ProjectID: "other"})
	if missing["status"] != "runtime_missing" {
		t.Fatalf("missing runtime: %#v", missing)
	}
}

func TestAbsentRuntimeDoesNotTouchFilesystem(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	svc := New(missingRoot, "", map[string]Registration{
		"other": {ProjectID: "other", Lifecycle: "active"},
	})
	result := svc.ProjectRuntime(QueryArgs{ProjectID: "other"})
	if result["status"] != "runtime_missing" {
		t.Fatalf("absent runtime: %#v", result)
	}
	if _, err := os.Stat(missingRoot); !os.IsNotExist(err) {
		t.Fatalf("runtime lookup created or touched missing root: %v", err)
	}
}

func TestClosedCheckKindsRejectCommandMaterial(t *testing.T) {
	src, _ := runtimeFixture(t)
	dst := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(src, "projects", "demo", "runtime.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "  - kind: git_status\n    target: repository", "  - kind: git_status\n    target: repository\n    command: echo forbidden", 1))
	path := filepath.Join(dst, "projects", "demo")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "runtime.yaml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	runtimeGitInit(t, dst)
	head := runtimeGitOut(t, dst, "rev-parse", "HEAD")
	svc := New(dst, head, map[string]Registration{
		"demo": {ProjectID: "demo", Lifecycle: "active", RuntimeResource: "projects/demo/runtime.yaml"},
	})
	result := svc.ProjectRuntime(QueryArgs{ProjectID: "demo"})
	if result["status"] != "error" || result["code"] != "runtime_failed" || !strings.Contains(result["message"].(string), "runtime_forbidden_field") {
		t.Fatalf("command-shaped runtime material was not rejected: %#v", result)
	}
}

func TestClosedCheckKindSet(t *testing.T) {
	expected := []string{"directory_exists", "docker_compose_services", "failed_units", "file_exists", "git_head", "git_status", "systemd_units"}
	actual := ClosedValues().CheckKinds
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("closed check kind set changed: got %v want %v", actual, expected)
	}
}

func TestClosedValuesExposeSortedValidatorSets(t *testing.T) {
	values := ClosedValues()
	checks := [][]string{
		values.EnvironmentKinds, values.Purposes, values.Capabilities,
		values.SupervisorKinds, values.RelationKinds, values.CheckKinds,
	}
	for _, got := range checks {
		if !sort.StringsAreSorted(got) {
			t.Fatalf("closed values are not sorted: %v", got)
		}
	}
}

func TestValidateManifestDetailedAggregatesIndependentFindings(t *testing.T) {
	raw := runtimeFixtureRaw(t)
	text := string(raw)
	text = strings.Replace(text, "  host_id: local\n", "  host_id: Local Linux\n", 1)
	text = strings.Replace(text, "  kind: workspace\n", "  kind: virtual_machine\n", 1)
	text = strings.Replace(text, "  purpose: source_development\n", "  purpose: coding\n", 1)
	text = strings.Replace(text, "  workdir: /workspace/demo\n", "  workdir: workspace/demo\n", 1)
	text = strings.Replace(text, "  - build\n", "  - build\n  - root\n", 1)
	report := ValidateManifestDetailed([]byte(text), "demo")
	if report.Valid || len(report.Errors) < 5 {
		t.Fatalf("expected aggregated findings, got %#v", report)
	}
	want := map[string]bool{
		"environments[0].host_id":         false,
		"environments[0].kind":            false,
		"environments[0].purpose":         false,
		"environments[0].workdir":         false,
		"environments[0].capabilities[3]": false,
	}
	previous := ""
	for _, finding := range report.Errors {
		if previous != "" && finding.Path < previous {
			t.Fatalf("findings not path-sorted: %#v", report.Errors)
		}
		previous = finding.Path
		if _, ok := want[finding.Path]; ok {
			want[finding.Path] = true
		}
	}
	for path, seen := range want {
		if !seen {
			t.Errorf("missing finding for %s: %#v", path, report.Errors)
		}
	}
}

func TestValidateManifestDetailedStopsDependentSupervisorCascade(t *testing.T) {
	raw := runtimeFixtureRaw(t)
	text := strings.Replace(string(raw), "    kind: none\n", "    kind: systemd\n", 1)
	report := ValidateManifestDetailed([]byte(text), "demo")
	if report.Valid {
		t.Fatal("invalid supervisor kind unexpectedly passed")
	}
	count := 0
	for _, finding := range report.Errors {
		if strings.HasPrefix(finding.Path, "environments[0].supervisor") {
			count++
			if finding.Path != "environments[0].supervisor.kind" {
				t.Fatalf("dependent supervisor cascade: %#v", report.Errors)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one supervisor finding, got %#v", report.Errors)
	}
}

func TestValidateManifestDetailedDoesNotEchoForbiddenSecretValue(t *testing.T) {
	raw := runtimeFixtureRaw(t)
	const secret = "do-not-echo-this-secret"
	text := strings.Replace(string(raw), "  description: sanitized development workspace\n", "  description: sanitized development workspace\n  api_key: "+secret+"\n", 1)
	report := ValidateManifestDetailed([]byte(text), "demo")
	if report.Valid {
		t.Fatal("forbidden secret field unexpectedly passed")
	}
	found := false
	for _, finding := range report.Errors {
		if strings.Contains(finding.Message, secret) {
			t.Fatalf("secret value echoed in finding: %#v", finding)
		}
		if finding.Code == "runtime_forbidden_field" && finding.Path == "environments[0].api_key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing forbidden-field finding: %#v", report.Errors)
	}
}

func TestValidateManifestCompatibilityUsesFirstDetailedFinding(t *testing.T) {
	raw := runtimeFixtureRaw(t)
	text := strings.Replace(string(raw), "  host_id: local\n", "  host_id: Local Linux\n", 1)
	err := ValidateManifest([]byte(text), "demo")
	if err == nil || !strings.HasPrefix(err.Error(), "runtime_host_id:") {
		t.Fatalf("unexpected compatibility error: %v", err)
	}
}

func TestValidateManifestDetailedMatchesStrictParser(t *testing.T) {
	base := string(runtimeFixtureRaw(t))
	cases := []struct {
		name   string
		mutate func(string) string
	}{
		{name: "valid", mutate: func(s string) string { return s }},
		{name: "host", mutate: func(s string) string { return strings.Replace(s, "  host_id: local\n", "  host_id: Local Linux\n", 1) }},
		{name: "environment kind", mutate: func(s string) string {
			return strings.Replace(s, "  kind: workspace\n", "  kind: virtual_machine\n", 1)
		}},
		{name: "purpose", mutate: func(s string) string {
			return strings.Replace(s, "  purpose: source_development\n", "  purpose: coding\n", 1)
		}},
		{name: "capability", mutate: func(s string) string { return strings.Replace(s, "  - build\n", "  - build\n  - root\n", 1) }},
		{name: "relative workdir", mutate: func(s string) string {
			return strings.Replace(s, "  workdir: /workspace/demo\n", "  workdir: workspace/demo\n", 1)
		}},
		{name: "repository branch", mutate: func(s string) string { return strings.Replace(s, "    branch: main\n", "    branch: ''\n", 1) }},
		{name: "supervisor discriminator", mutate: func(s string) string { return strings.Replace(s, "    kind: none\n", "    kind: systemd\n", 1) }},
		{name: "supervisor extra field", mutate: func(s string) string {
			return strings.Replace(s, "    kind: none\n", "    kind: none\n    containers: [demo]\n", 1)
		}},
		{name: "systemd missing units", mutate: func(s string) string {
			return strings.Replace(s, "    kind: systemd_system\n    units:\n    - example.service\n", "    kind: systemd_system\n", 1)
		}},
		{name: "check discriminator", mutate: func(s string) string { return strings.Replace(s, "  - kind: git_status\n", "  - kind: shell\n", 1) }},
		{name: "check target", mutate: func(s string) string {
			return strings.Replace(s, "    target: repository\n", "    target: workdir\n", 1)
		}},
		{name: "relation kind", mutate: func(s string) string { return strings.Replace(s, "- kind: deploys_to\n", "- kind: copies_to\n", 1) }},
		{name: "relation reference", mutate: func(s string) string {
			return strings.Replace(s, "  to: arm-production\n", "  to: missing-environment\n", 1)
		}},
		{name: "duplicate environment", mutate: func(s string) string {
			return strings.Replace(s, "- environment_id: arm-production\n", "- environment_id: local-workspace\n", 1)
		}},
		{name: "verification timestamp", mutate: func(s string) string {
			return strings.Replace(s, "  verified_at: '2026-08-11T00:00:00Z'\n", "  verified_at: yesterday\n", 1)
		}},
		{name: "verification duration", mutate: func(s string) string {
			return strings.Replace(s, "  stale_after: P99999D\n", "  stale_after: PT1H\n", 1)
		}},
		{name: "verification evidence", mutate: func(s string) string {
			return strings.Replace(s, "  - memory://projects/demo/handoff\n", "  - https://example.invalid/evidence\n", 1)
		}},
		{name: "forbidden field", mutate: func(s string) string {
			return strings.Replace(s, "  description: sanitized development workspace\n", "  description: sanitized development workspace\n  api_key: hidden\n", 1)
		}},
		{name: "top level field", mutate: func(s string) string {
			return strings.Replace(s, "project_id: demo\n", "project_id: demo\nextra: value\n", 1)
		}},
		{name: "malformed yaml", mutate: func(s string) string { return s + "\n  broken: [\n" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.mutate(base))
			report := ValidateManifestDetailed(raw, "demo")
			_, strictErr := parseManifest(raw, "demo")
			strictValid := strictErr == nil
			if report.Valid != strictValid {
				t.Fatalf("validator/parser divergence: detailed_valid=%v strict_valid=%v detailed=%#v strict_err=%v", report.Valid, strictValid, report.Errors, strictErr)
			}
		})
	}
}

func TestSingleEnvironmentDefaultsIDAndHostIsOptionalUserDefined(t *testing.T) {
	raw := runtimeFixtureRaw(t)
	text := string(raw)
	text = strings.Replace(text, "- environment_id: local-workspace\n  host_id: local\n", "- host_id: laptop\n", 1)
	// Keep the fixture single-environment so omission is unambiguous.
	cut := strings.Index(text, "- environment_id: arm-production\n")
	if cut < 0 {
		t.Fatal("second environment fixture not found")
	}
	relations := strings.Index(text, "relations:\n")
	if relations < cut {
		t.Fatal("relations fixture not found")
	}
	text = text[:cut] + text[relations:]
	text = strings.Replace(text, "relations:\n- kind: deploys_to\n  from: local-workspace\n  to: arm-production\n", "relations: []\n", 1)
	report := ValidateManifestDetailed([]byte(text), "demo")
	if !report.Valid {
		t.Fatalf("single environment shorthand rejected: %#v", report.Errors)
	}
	manifest, err := parseManifest([]byte(text), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Environments) != 1 || manifest.Environments[0]["environment_id"] != "default" || manifest.Environments[0]["host_id"] != "laptop" {
		t.Fatalf("normalized environment: %#v", manifest.Environments)
	}
}

func TestMultiEnvironmentRequiresExplicitEnvironmentIDs(t *testing.T) {
	raw := runtimeFixtureRaw(t)
	text := strings.Replace(string(raw), "- environment_id: local-workspace\n", "- ", 1)
	report := ValidateManifestDetailed([]byte(text), "demo")
	if report.Valid {
		t.Fatal("multi-environment manifest accepted missing environment_id")
	}
}

func TestRuntimePackageHasNoExecutionOrNetworkImports(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"os/exec": true, "net": true, "net/http": true}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, "\"")
				if forbidden[path] || strings.HasPrefix(path, "net/") {
					t.Fatalf("runtime read package must not import execution/network package %q", path)
				}
			}
			ast.Inspect(f, func(ast.Node) bool { return true })
		}
	}
}

func runtimeFixture(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "runtime-read")
	dst := filepath.Join(t.TempDir(), "vault")
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	}); err != nil {
		t.Fatal(err)
	}
	runtimeGitInit(t, dst)
	return dst, runtimeGitOut(t, dst, "rev-parse", "HEAD")
}

func runtimeFixtureRaw(t *testing.T) []byte {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "conformance", "fixtures", "runtime-read", "projects", "demo", "runtime.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func runtimeGitInit(t *testing.T, root string) {
	t.Helper()
	runtimeGit(t, root, "init", "-q", "-b", "main")
	runtimeGit(t, root, "add", ".")
	cmd := exec.Command("git", "-C", root, "commit", "-q", "-m", "fixture")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}
func runtimeGit(t *testing.T, root string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
func runtimeGitOut(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
