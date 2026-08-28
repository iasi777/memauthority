// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/iasi777/v-memory/internal/authorityscan"
	"github.com/iasi777/v-memory/internal/vaultread"
	"gopkg.in/yaml.v3"
)

type primaryDeclaration struct {
	NodeID string `yaml:"node_id"`
	Epoch  int    `yaml:"epoch"`
}

func invalidProjectIDPath(projectID string) bool {
	value := strings.TrimSpace(projectID)
	return value != projectID || !vaultread.ValidProjectID(value)
}

func containsHighConfidenceSecret(text string) bool {
	return authorityscan.ContainsHighConfidenceSecret(text)
}

func payloadContainsHighConfidenceSecret(value any) bool {
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return authorityscan.ContainsHighConfidenceSecret(string(raw))
}

func (s *Service) primaryState() (bool, map[string]any) {
	if !s.requirePrimary {
		return false, map[string]any{"state": "missing", "node_id": nil, "epoch": nil}
	}
	raw, err := os.ReadFile(s.root + string(os.PathSeparator) + "PRIMARY")
	if err != nil {
		return false, map[string]any{
			"state": "missing", "node_id": nil, "epoch": nil,
			"local_node_id": s.localNodeID, "expected_epoch": s.primaryEpoch,
		}
	}
	var decl primaryDeclaration
	if err := yaml.Unmarshal(raw, &decl); err != nil || strings.TrimSpace(decl.NodeID) == "" || decl.Epoch < 1 {
		return false, map[string]any{
			"state": "invalid", "node_id": nil, "epoch": nil,
			"local_node_id": s.localNodeID, "expected_epoch": s.primaryEpoch,
		}
	}
	if decl.NodeID == s.localNodeID && decl.Epoch == s.primaryEpoch {
		return true, map[string]any{"state": "active", "node_id": decl.NodeID, "epoch": decl.Epoch}
	}
	return false, map[string]any{
		"state": "mismatch", "node_id": decl.NodeID, "epoch": decl.Epoch,
		"local_node_id": s.localNodeID, "expected_epoch": s.primaryEpoch,
	}
}

func (s *Service) writePreflight() map[string]any {
	if !s.writeEnabled {
		return mutationError("write_disabled", "", "write operations are disabled")
	}
	if s.legacyIndex {
		return mutationError("read_only", "migration_required", "lifecycle migration is required before ordinary writes")
	}
	primaryActive, _ := s.primaryState()
	if s.requirePrimary && !primaryActive {
		return mutationError("read_only", "", "primary node or epoch is not active")
	}
	head, err := gitHead(s.root)
	if err != nil {
		return mutationError("mutation_failed", "git_head", err.Error())
	}
	if s.indexHead == "" || s.indexHead != head {
		return mutationError("read_only", "stale_index", "rebuildable index is stale")
	}
	clean, dirty, err := gitWorktreeState(s.root)
	if err != nil {
		return mutationError("mutation_failed", "workspace", err.Error())
	}
	if !clean {
		return map[string]any{
			"status": "error", "code": "read_only", "rule": "dirty_workspace",
			"message": "Authority worktree is dirty", "dirty_paths": dirty, "audit_state": "recorded",
		}
	}
	return nil
}

func (s *Service) writeModeState() (string, bool, bool, []string, string, map[string]any) {
	head, _ := gitHead(s.root)
	clean, dirty, _ := gitWorktreeState(s.root)
	primaryActive, primary := s.primaryState()
	indexState := "current"
	if s.indexHead == "" || s.indexHead != head {
		indexState = "stale"
	}
	readOnly := !s.writeEnabled
	writeMode := "ready"
	if !clean || indexState != "current" || (s.requirePrimary && !primaryActive) {
		readOnly = true
		writeMode = "degraded_read_only"
	}
	return writeMode, readOnly, primaryActive, dirty, indexState, primary
}

func (s *Service) maybeInjectExternalRefMovement() error {
	if s.testFault != "external_ref_movement_before_cas" || s.faultInjected {
		return nil
	}
	s.faultInjected = true
	env := append(os.Environ(),
		"GIT_AUTHOR_DATE=2026-08-11T00:05:00Z",
		"GIT_COMMITTER_DATE=2026-08-11T00:05:00Z",
	)
	if _, err := gitInput(s.root, env, nil, "commit", "--allow-empty", "-m", "external: conformance ref movement"); err != nil {
		return fmt.Errorf("inject external ref movement: %w", err)
	}
	return nil
}

func (s *Service) writePreflightLegacyMigration() map[string]any {
	if !s.writeEnabled {
		return mutationError("write_disabled", "", "write operations are disabled")
	}
	primaryActive, _ := s.primaryState()
	if s.requirePrimary && !primaryActive {
		return mutationError("read_only", "", "primary node or epoch is not active")
	}
	head, err := gitHead(s.root)
	if err != nil {
		return mutationError("mutation_failed", "git_head", err.Error())
	}
	if s.indexHead == "" || s.indexHead != head {
		return mutationError("read_only", "stale_index", "rebuildable index is stale")
	}
	clean, dirty, err := gitWorktreeState(s.root)
	if err != nil {
		return mutationError("mutation_failed", "workspace", err.Error())
	}
	if !clean {
		return map[string]any{"status": "error", "code": "read_only", "rule": "dirty_workspace", "message": "Authority worktree is dirty", "dirty_paths": dirty, "audit_state": "recorded"}
	}
	if !s.legacyIndex {
		return mutationError("validation_failed", "migration_state", "legacy lifecycle INDEX is required")
	}
	return nil
}
