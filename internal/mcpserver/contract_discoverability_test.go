// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/iasi777/v-memory/internal/runtimeview"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInstructionsAreProgressiveAndSelfContained(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := inMemorySession(t, app)
	init := session.InitializeResult()
	if init == nil {
		t.Fatal("initialize result is missing")
	}
	instructions := init.Instructions
	if len(instructions) == 0 || len(instructions) > 512 {
		t.Fatalf("instructions length=%d want 1..512: %q", len(instructions), instructions)
	}
	for _, term := range []string{"long-term memory", "not already available", "declarative runtime", "does not prove live runtime health", "Authority writes", "not machine or filesystem tools"} {
		if !strings.Contains(instructions, term) {
			t.Fatalf("instructions lack %q: %q", term, instructions)
		}
	}
	lower := strings.ToLower(instructions)
	for _, forbidden := range []string{"memory_route", "memory_search", "memory_read", "always recall", "must recall", "route -> search", "route → search"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("instructions contain fixed recall ritual %q: %q", forbidden, instructions)
		}
	}
}

func TestToolTitlesAndPairwiseDescriptions(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := inMemorySession(t, app)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
		if tool.Annotations == nil || strings.TrimSpace(tool.Annotations.Title) == "" {
			t.Errorf("tool %s lacks searchable title: %#v", tool.Name, tool.Annotations)
		}
	}
	if len(tools) != 16 {
		t.Fatalf("tool count=%d want 16", len(tools))
	}
	assertDescriptionTerms(t, tools["memory_route"], "uncertain", "exact", "alias", "already known")
	assertDescriptionTerms(t, tools["memory_search"], "known project", "exact memory resource", "does not resolve uncertain project identity")
	assertDescriptionTerms(t, tools["memory_project_runtime"], "declarative runtime", "does not execute checks", "MemAuthority service", "live deployment health")
	assertDescriptionTerms(t, tools["memory_status"], "MemAuthority service", "Authority workspace", "does not report a project's deployment")
	assertDescriptionTerms(t, tools["memory_update_handoff"], "direct replacement", "Prefer memory_update_sections")
	assertDescriptionTerms(t, tools["memory_update_sections"], "ordered typed section edits", "Prefer memory_update_handoff")
}

func TestRuntimeUpdateSelectedToolContractUsesClosedValueTruth(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := inMemorySession(t, app)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var tool *mcp.Tool
	for _, candidate := range listed.Tools {
		if candidate.Name == "memory_update_runtime" {
			tool = candidate
			break
		}
	}
	if tool == nil {
		t.Fatal("memory_update_runtime is missing")
	}
	values := runtimeview.ClosedValues()
	for _, value := range append(append([]string{}, values.EnvironmentKinds...), values.CheckKinds...) {
		if !strings.Contains(tool.Description, value) {
			t.Errorf("runtime tool description lacks searchable closed value %q: %q", value, tool.Description)
		}
	}

	schema := mustObject(t, tool.InputSchema, "memory_update_runtime schema")
	top := mustProperties(t, schema, "memory_update_runtime schema")
	for _, field := range []string{"expected_index_revision", "expected_resource_revision", "client_idempotency_key"} {
		if descriptionOf(t, top[field], field) == "" {
			t.Errorf("top-level runtime field %s lacks selected-tool guidance", field)
		}
	}
	runtimeSchema := mustObject(t, top["runtime"], "runtime")
	runtimeProps := mustProperties(t, runtimeSchema, "runtime")
	environments := mustObject(t, runtimeProps["environments"], "runtime.environments")
	envItems := mustObject(t, environments["items"], "runtime.environments.items")
	envProps := mustProperties(t, envItems, "runtime.environments.items")
	required, _ := envItems["required"].([]any)
	for _, field := range []string{"environment_id", "host_id"} {
		for _, raw := range required {
			if raw == field {
				t.Fatalf("%s unexpectedly required in runtime environment input", field)
			}
		}
	}
	for _, term := range []string{"optional", "user-defined"} {
		if !strings.Contains(strings.ToLower(descriptionOf(t, envProps["host_id"], "host_id")), term) {
			t.Errorf("host_id description lacks %q", term)
		}
	}
	assertDescriptionContainsValues(t, envProps["kind"], "environment kind", values.EnvironmentKinds)
	assertDescriptionContainsValues(t, envProps["purpose"], "purpose", values.Purposes)
	assertDescriptionContainsValues(t, envProps["capabilities"], "capabilities", values.Capabilities)
	for _, field := range []string{"host_id", "kind", "purpose", "capabilities"} {
		if _, hasEnum := mustObject(t, envProps[field], field)["enum"]; hasEnum {
			t.Fatalf("nested %s unexpectedly gained SDK enum and would bypass aggregate validation", field)
		}
	}

	supervisor := mustObject(t, envProps["supervisor"], "supervisor")
	supervisorProps := mustProperties(t, supervisor, "supervisor")
	assertDescriptionContainsValues(t, supervisorProps["kind"], "supervisor.kind", values.SupervisorKinds)
	checks := mustObject(t, envProps["checks"], "checks")
	checkItems := mustObject(t, checks["items"], "checks.items")
	checkProps := mustProperties(t, checkItems, "checks.items")
	assertDescriptionContainsValues(t, checkProps["kind"], "check.kind", values.CheckKinds)
	if _, hasEnum := mustObject(t, checkProps["kind"], "check.kind")["enum"]; hasEnum {
		t.Fatal("nested check.kind unexpectedly gained SDK enum")
	}
	relations := mustObject(t, runtimeProps["relations"], "relations")
	relationItems := mustObject(t, relations["items"], "relations.items")
	relationProps := mustProperties(t, relationItems, "relations.items")
	assertDescriptionContainsValues(t, relationProps["kind"], "relation.kind", values.RelationKinds)
}

func TestMemoryMarkVerifiedSchemaDeclaresVerificationLevels(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	session := inMemorySession(t, app)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var tool *mcp.Tool
	for _, candidate := range listed.Tools {
		if candidate.Name == "memory_mark_verified" {
			tool = candidate
			break
		}
	}
	if tool == nil {
		t.Fatal("memory_mark_verified is missing")
	}
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema type %T", tool.InputSchema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input schema properties: %#v", schema)
	}
	level, ok := properties["verification_level"].(map[string]any)
	if !ok {
		t.Fatalf("verification_level schema: %#v", properties["verification_level"])
	}
	enum, ok := level["enum"].([]any)
	if !ok || len(enum) != 2 || enum[0] != "local" || enum[1] != "runtime" {
		t.Fatalf("verification_level enum=%#v want [local runtime]", level["enum"])
	}
	description, _ := level["description"].(string)
	for _, term := range []string{"local", "source code", "configuration", "documentation", "static", "runtime", "running environment", "deployment", "business flow"} {
		if !strings.Contains(description, term) {
			t.Fatalf("verification_level description lacks %q: %q", term, description)
		}
	}
}

func TestChecklistContractIsDiscoverableFromExistingTools(t *testing.T) {
	root := fixtureRepo(t)
	app, err := Open(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	session := inMemorySession(t, app)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	read := tools["memory_read"]
	sections := tools["memory_update_sections"]
	if read == nil || sections == nil {
		t.Fatalf("checklist tools missing: read=%#v sections=%#v", read, sections)
	}
	if !strings.Contains(read.Description, "checklist_items") || !strings.Contains(read.Description, "已知问题 / 待办") {
		t.Fatalf("memory_read does not advertise checklist metadata: %q", read.Description)
	}
	if !strings.Contains(sections.Description, "checklist_remove") || !strings.Contains(sections.Description, "checklist_set_checked") || !strings.Contains(sections.Description, "item_ref") {
		t.Fatalf("memory_update_sections does not advertise checklist operations: %q", sections.Description)
	}

	schema, ok := sections.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema type %T", sections.InputSchema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input schema properties: %#v", schema)
	}
	operations, ok := properties["operations"].(map[string]any)
	if !ok || !strings.Contains(operations["description"].(string), "checklist_remove") {
		t.Fatalf("operations schema lacks checklist description: %#v", properties["operations"])
	}
	items, ok := operations["items"].(map[string]any)
	if !ok {
		t.Fatalf("operations item schema: %#v", operations)
	}
	itemProperties, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("operations item properties: %#v", items)
	}
	for _, field := range []string{"item_ref", "checked"} {
		property, ok := itemProperties[field].(map[string]any)
		if !ok {
			t.Fatalf("%s schema missing: %#v", field, itemProperties)
		}
		description, _ := property["description"].(string)
		if description == "" {
			t.Fatalf("%s schema lacks description: %#v", field, property)
		}
	}
}

func assertDescriptionTerms(t *testing.T, tool *mcp.Tool, terms ...string) {
	t.Helper()
	if tool == nil {
		t.Fatal("expected tool is missing")
	}
	for _, term := range terms {
		if !strings.Contains(tool.Description, term) {
			t.Errorf("%s description lacks %q: %q", tool.Name, term, tool.Description)
		}
	}
}

func mustObject(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s type %T want object: %#v", label, value, value)
	}
	return m
}

func mustProperties(t *testing.T, schema map[string]any, label string) map[string]any {
	t.Helper()
	return mustObject(t, schema["properties"], label+".properties")
}

func descriptionOf(t *testing.T, value any, label string) string {
	t.Helper()
	m := mustObject(t, value, label)
	description, _ := m["description"].(string)
	return description
}

func assertDescriptionContainsValues(t *testing.T, value any, label string, values []string) {
	t.Helper()
	description := descriptionOf(t, value, label)
	if description == "" {
		t.Errorf("%s lacks description", label)
		return
	}
	for _, item := range values {
		if !strings.Contains(description, item) {
			t.Errorf("%s description lacks closed value %q: %q", label, item, description)
		}
	}
}
