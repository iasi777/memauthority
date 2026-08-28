// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iasi777/v-memory/internal/textcanon"
)

// legacyOperationFingerprint reproduces the frozen Python operation_fingerprint
// used by the production writer. It exists only for the bounded legacy migration
// replay horizon; new Go operations continue to use the native payload hash.
func legacyOperationFingerprint(projectID, role, operation string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", err
	}
	value = canonicalLegacyValue(value)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	canonicalJSON := strings.TrimSuffix(buf.String(), "\n")
	material := strings.Join([]string{projectID, role, operation, canonicalJSON}, "\n")
	sum := sha256.Sum256([]byte(material))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func canonicalLegacyValue(value any) any {
	switch typed := value.(type) {
	case string:
		return textcanon.CanonicalText(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = canonicalLegacyValue(typed[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = canonicalLegacyValue(item)
		}
		return out
	default:
		return value
	}
}

func legacyEvidence(values []EvidenceInput) []EvidenceInput {
	if values == nil {
		return []EvidenceInput{}
	}
	return values
}

func legacyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (s *Service) callerKeyDisposition(callerKeyHash, payloadHash, projectID, role, operation string, legacyPayload any) (map[string]any, bool) {
	if callerKeyHash == "" {
		return nil, false
	}
	existing, err := s.findByCallerKey(callerKeyHash)
	if err != nil {
		return mutationError("mutation_failed", "idempotency_key", err.Error()), true
	}
	if existing == nil {
		return nil, false
	}
	if existing.LegacyFingerprint != "" {
		fingerprint, err := legacyOperationFingerprint(projectID, role, operation, legacyPayload)
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
