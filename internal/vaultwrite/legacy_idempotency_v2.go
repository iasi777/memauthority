// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"fmt"
	"strings"

	"github.com/iasi777/memauthority/internal/textcanon"
)

func p5LegacyCleanText(value string) string {
	return strings.TrimSpace(textcanon.CanonicalText(value))
}

func p5LegacyEvidence(values []EvidenceInput) []EvidenceInput {
	if values == nil {
		return []EvidenceInput{}
	}
	out := make([]EvidenceInput, len(values))
	for i, value := range values {
		out[i] = EvidenceInput{Kind: value.Kind, Instance: value.Instance, Description: p5LegacyCleanText(value.Description)}
	}
	return out
}

func p5LegacyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = p5LegacyCleanText(value)
	}
	return out
}

func p5LegacySectionUpdates(values []SectionUpdateInput) []map[string]any {
	out := make([]map[string]any, len(values))
	for i, value := range values {
		out[i] = map[string]any{"heading": p5LegacyCleanText(value.Heading), "replacement": p5LegacyCleanText(value.Replacement)}
	}
	return out
}

func p5LegacySectionOperations(values []SectionOperationInput) []map[string]any {
	out := make([]map[string]any, len(values))
	for i, value := range values {
		var content any
		if value.Content != nil {
			clean := p5LegacyCleanText(*value.Content)
			if value.Operation != "delete" || clean != "" {
				content = clean
			}
		}
		entry := map[string]any{"operation": value.Operation, "heading": p5LegacyCleanText(value.Heading), "content": content}
		if value.ItemRef != nil {
			entry["item_ref"] = p5LegacyCleanText(*value.ItemRef)
		}
		if value.Checked != nil {
			entry["checked"] = *value.Checked
		}
		out[i] = entry
	}
	return out
}

func p5LegacyOptionalString(value *string) any {
	if value == nil {
		return nil
	}
	return p5LegacyCleanText(*value)
}

func p5LegacyRelated(value string) any {
	if value == "" {
		return nil
	}
	return p5LegacyCleanText(value)
}

func p5LegacyLifecycleManifest(value LifecycleMigrationManifestInput) map[string]any {
	projects := make([]map[string]any, len(value.Projects))
	for i, project := range value.Projects {
		projects[i] = map[string]any{
			"project_id":       p5LegacyCleanText(project.ProjectID),
			"source_lifecycle": project.SourceLifecycle,
			"target_lifecycle": project.TargetLifecycle,
		}
	}
	return map[string]any{
		"schema_version":          value.SchemaVersion,
		"expected_index_revision": value.ExpectedIndexRevision,
		"archive_reason":          p5LegacyCleanText(value.ArchiveReason),
		"projects":                projects,
	}
}

func p5LegacyCallerKeyHash(value string) string {
	clean := p5LegacyCleanText(value)
	if clean == "" {
		return ""
	}
	return hashText(clean)
}

func (s *Service) callerKeyDispositionV2(callerKeyHash, rawCallerKey, payloadHash, projectID, role, operation string, legacyPayload any) (map[string]any, bool) {
	if callerKeyHash == "" {
		return nil, false
	}
	existing, err := s.findByCallerKey(callerKeyHash)
	if err != nil {
		return mutationError("mutation_failed", "idempotency_key", err.Error()), true
	}
	if existing == nil {
		legacyHash := p5LegacyCallerKeyHash(rawCallerKey)
		if legacyHash != "" && legacyHash != callerKeyHash {
			candidate, lookupErr := s.findByCallerKey(legacyHash)
			if lookupErr != nil {
				return mutationError("mutation_failed", "idempotency_key", lookupErr.Error()), true
			}
			if candidate != nil && candidate.LegacyFingerprint != "" {
				existing = candidate
			}
		}
	}
	if existing == nil {
		return nil, false
	}
	if existing.LegacyFingerprint != "" {
		fingerprint, err := legacyOperationFingerprint(p5LegacyCleanText(projectID), role, operation, legacyPayload)
		if err != nil {
			return mutationError("mutation_failed", "idempotency_key", fmt.Sprintf("compute legacy operation fingerprint: %v", err)), true
		}
		if fingerprint != existing.LegacyFingerprint {
			return mutationError("validation_failed", "idempotency_key", "client_idempotency_key was already used for different content"), true
		}
	} else if existing.PayloadHash != payloadHash {
		return mutationError("validation_failed", "idempotency_key", "client_idempotency_key was already used for different content"), true
	}
	return replayResult(existing.Result), true
}
