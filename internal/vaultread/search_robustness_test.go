// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchQueryModesAndFallbackBoundaries(t *testing.T) {
	root := searchFixtureRepo(t)
	svc := openSearchService(t, root, filepath.Join(t.TempDir(), "projection.sqlite"))
	defer svc.Close()

	tests := []struct {
		name      string
		query     string
		matchType string
		hitCount  int
		heading   string
	}{
		{"natural language Chinese", "这个项目哪里包含中文检索词？", "lexical_overlap", 1, "交接 / 中文检索"},
		{"natural language English punctuation", "Where does alphaneedle appear?", "lexical_overlap", 1, "交接 / English Needle"},
		{"exact phrase compatibility", "中文检索词", "fts5_trigram", 1, "交接 / 中文检索"},
		{"short substring compatibility", "检索", "like_short_query", 1, "交接 / 中文检索"},
		{"frontmatter exact compatibility", "Search fixture handoff", "fts5_trigram", 1, ""},
		{"fallback ignores frontmatter", "Where is Search fixture handoff metadata?", "lexical_overlap", 0, ""},
		{"fallback preserves zero hit", "marsweatherforecast", "lexical_overlap", 0, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := svc.Search(SearchArgs{Query: test.query, ProjectID: "alpha"})
			if result["status"] != "ok" || result["match_type"] != test.matchType {
				t.Fatalf("search=%#v", result)
			}
			hits := result["results"].([]map[string]any)
			if len(hits) != test.hitCount {
				t.Fatalf("hits=%d want %d: %#v", len(hits), test.hitCount, result)
			}
			if test.hitCount > 0 && hits[0]["heading"] != test.heading {
				t.Fatalf("heading=%#v want %q: %#v", hits[0]["heading"], test.heading, result)
			}
		})
	}
}

func TestSearchLexicalFallbackSupportsCrossProjectScope(t *testing.T) {
	root := searchFixtureRepo(t)
	indexPath := filepath.Join(root, "INDEX.yaml")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	index = append(index, []byte("  beta:\n    description: Second search fixture\n    lifecycle: active\n    aliases: []\n    runtime_resource: null\n    last_updated: '2026-08-11'\n    last_verified: '2026-08-11'\n")...)
	if err := os.WriteFile(indexPath, index, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	beta := `---
项目: beta
创建: 2026-08-11
最后更新: 2026-08-11
最后核验: 2026-08-11
说明: Cross-project search fixture.
---
# 交接

## Beta Needle

The unique token betaneedle appears only in this project.
`
	if err := os.WriteFile(filepath.Join(root, "beta", "交接.md"), []byte(beta), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "INDEX.yaml", "beta/交接.md")
	cmd := gitCommitCommand(root, "add beta search fixture")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit beta fixture: %v: %s", err, out)
	}

	svc := openSearchService(t, root, filepath.Join(t.TempDir(), "projection.sqlite"))
	defer svc.Close()
	result := svc.Search(SearchArgs{Query: "Where is betaneedle documented?", CrossProject: true})
	if result["status"] != "ok" || result["match_type"] != "lexical_overlap" {
		t.Fatalf("search=%#v", result)
	}
	hits := result["results"].([]map[string]any)
	if len(hits) == 0 || hits[0]["project_id"] != "beta" || hits[0]["heading"] != "交接 / Beta Needle" {
		t.Fatalf("cross-project fallback missed beta: %#v", result)
	}
	scoped := svc.Search(SearchArgs{Query: "Where is betaneedle documented?", ProjectID: "alpha"})
	if scoped["status"] != "ok" {
		t.Fatalf("scoped search=%#v", scoped)
	}
	for _, hit := range scoped["results"].([]map[string]any) {
		if hit["project_id"] == "beta" {
			t.Fatalf("scoped fallback leaked beta: %#v", scoped)
		}
	}
}

func TestLexicalOverlapCoverageRankingAndUnicode(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE sections (
		id INTEGER PRIMARY KEY, project_id TEXT, role TEXT, heading TEXT,
		line_start INTEGER, line_end INTEGER, content TEXT, section_hash TEXT,
		resource_revision TEXT, resource_uri TEXT, source_revision TEXT, search_text TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id int, heading, content string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO sections(id,project_id,role,heading,line_start,line_end,content,section_hash,resource_revision,resource_uri,source_revision,search_text)
			VALUES(?, 'alpha', 'handoff', ?, ?, ?, ?, 'h', 'r', 'u', 's', ?)`, id, heading, id, id, content, content)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(1, "Question Noise", "不知道这个东西放在哪里，回头再问。")
	insert(2, "API Secret", "API 密钥保存在 vault 的 secrets 目录。")
	insert(3, "Deploy Only", "deploy happens here")
	insert(4, "Deploy Token", "deploy token lives in vault")
	insert(5, "Unicode Upper", "МОСКВА находится здесь")
	insert(6, "Unicode Title", "Москва находится здесь")
	insert(7, "Cafe NFC", "café lives here")
	insert(8, "Current Port", "当前状态：API 监听 7906。")
	insert(9, "Historical Migration", "API 从 7800 迁移到 7905。")
	insert(10, "Historical Switch", "完成 7906 切换。")
	p := &projection{db: db}

	assertTop := func(query, heading string) {
		t.Helper()
		results, err := p.searchLexicalOverlap(query, "alpha", false)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) == 0 || results[0]["heading"] != heading {
			t.Fatalf("query=%q top=%#v want %q", query, results, heading)
		}
	}
	assertTop("API密钥在哪里？", "API Secret")
	assertTop("Where is the deploy token.", "Deploy Token")
	assertTop("Atlas 在迁移到 7906 之前使用的端口是什么？", "Historical Migration")

	results, err := p.searchLexicalOverlap("Где находится МОСКВА?", "alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, result := range results {
		seen[result["heading"].(string)] = true
	}
	if !seen["Unicode Upper"] || !seen["Unicode Title"] {
		t.Fatalf("Unicode case-fold missed variants: %#v", results)
	}
	results, err = p.searchLexicalOverlap("cafe\u0301", "alpha", false)
	if err != nil || len(results) == 0 || results[0]["heading"] != "Cafe NFC" {
		t.Fatalf("NFC/NFD fallback mismatch: results=%#v err=%v", results, err)
	}
}

func TestLexicalOverlapFeaturesAreBoundedAndIgnoreWeakASCIIWords(t *testing.T) {
	plan := buildLexicalPlan("Where is API 12 recorded?")
	for _, want := range []string{"where", "api", "12", "recorded"} {
		if _, ok := plan.featureIndex[want]; !ok {
			t.Fatalf("missing %q in %#v", want, plan.featureIndex)
		}
	}
	if _, ok := plan.featureIndex["is"]; ok {
		t.Fatalf("short ASCII noise feature retained: %#v", plan.featureIndex)
	}
	punctuation := buildLexicalPlan("token. here... --- hyphen- slash/ v1.3.2 path/to/file.md memory_search")
	for _, want := range []string{"token", "here", "hyphen", "slash", "v1.3.2", "path/to/file.md", "memory_search"} {
		if _, ok := punctuation.featureIndex[want]; !ok {
			t.Fatalf("missing normalized token %q in %#v", want, punctuation.featureIndex)
		}
	}
	if _, ok := punctuation.featureIndex["---"]; ok {
		t.Fatalf("pure connector token retained: %#v", punctuation.featureIndex)
	}
	if got := len(buildLexicalPlan(strings.Repeat("检索", 300)).spans); got > maxLexicalFeatures {
		t.Fatalf("feature count=%d want <=%d", got, maxLexicalFeatures)
	}
	long := buildLexicalPlan(strings.Repeat("检索", maxLexicalQueryRunes+1000))
	for _, span := range long.spans {
		if span.end > maxLexicalQueryRunes {
			t.Fatalf("long-query feature escaped fallback bound: %#v", span)
		}
	}
}
