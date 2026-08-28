// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import "testing"

func TestRouteNFCUnicodeCasefoldArchivedAndAmbiguity(t *testing.T) {
	svc := &Service{projects: map[string]Project{
		"AgentDock":      {ID: "AgentDock", Lifecycle: "active"},
		"cafe-project":   {ID: "cafe-project", Lifecycle: "active", Aliases: []string{"Café"}},
		"street-project": {ID: "street-project", Lifecycle: "active", Aliases: []string{"Straße"}},
		"old-project":    {ID: "old-project", Lifecycle: "archived", Aliases: []string{"Legacy"}},
	}}
	cases := []struct {
		query, status, project string
	}{
		{"agentdock", "unique", "AgentDock"},
		{"Cafe\u0301", "unique", "cafe-project"},
		{"STRASSE", "unique", "street-project"},
		{"legacy", "archived", "old-project"},
		{"dock", "not_found", ""},
	}
	for _, tc := range cases {
		got := svc.Route(tc.query)
		if got["status"] != tc.status {
			t.Fatalf("route %q status: %#v", tc.query, got)
		}
		if tc.project != "" && got["project_id"] != tc.project {
			t.Fatalf("route %q project: %#v", tc.query, got)
		}
	}

	ambiguous := &Service{projects: map[string]Project{
		"alpha": {ID: "alpha", Lifecycle: "active", Aliases: []string{"Shared"}},
		"beta":  {ID: "beta", Lifecycle: "active", Aliases: []string{"shared"}},
	}}
	got := ambiguous.Route("SHARED")
	if got["status"] != "ambiguous" {
		t.Fatalf("ambiguity: %#v", got)
	}
	ids, ok := got["project_ids"].([]string)
	if !ok || len(ids) != 2 || ids[0] != "alpha" || ids[1] != "beta" {
		t.Fatalf("stable ambiguity ids: %#v", got)
	}
}
