// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/iasi777/memauthority/internal/pathguard"
	_ "modernc.org/sqlite"
)

const legacyReplayHorizon = 24 * time.Hour

var prefixedSHA256RE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type LegacyIdempotencyImportReport struct {
	SchemaVersion     int       `json:"schema_version"`
	AsOf              time.Time `json:"as_of"`
	HorizonHours      int       `json:"horizon_hours"`
	SourceRows        int       `json:"source_rows"`
	ExpiredRows       int       `json:"expired_rows"`
	UnkeyedActiveRows int       `json:"unkeyed_active_rows"`
	ImportedRows      int       `json:"imported_rows"`
	ExistingRows      int       `json:"existing_rows"`
	CallerKeys        int       `json:"caller_keys"`
	VaultHead         string    `json:"vault_head"`
}

// ImportLegacyPythonIdempotencyWindow converts only still-active frozen-Python
// caller-key records into Go durable operation records. The full legacy index
// remains an audit/rollback artifact; expired/unkeyed rows are not promoted into
// Go's indefinite caller-key namespace.
func ImportLegacyPythonIdempotencyWindow(vaultRoot, pythonIndexDB, stateDir string, asOf time.Time) (LegacyIdempotencyImportReport, error) {
	report := LegacyIdempotencyImportReport{SchemaVersion: 1, AsOf: asOf.UTC(), HorizonHours: int(legacyReplayHorizon / time.Hour)}
	if asOf.IsZero() {
		return report, fmt.Errorf("as-of time is required")
	}
	vaultAbs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return report, fmt.Errorf("resolve Vault root: %w", err)
	}
	stateAbs, err := filepath.Abs(stateDir)
	if err != nil {
		return report, fmt.Errorf("resolve state directory: %w", err)
	}
	inside, err := pathguard.Contains(vaultAbs, stateAbs)
	if err != nil {
		return report, fmt.Errorf("validate state directory containment: %w", err)
	}
	if inside {
		return report, fmt.Errorf("state directory must be outside Vault Authority")
	}
	head, err := gitHead(vaultAbs)
	if err != nil {
		return report, fmt.Errorf("read Vault HEAD: %w", err)
	}
	report.VaultHead = head

	indexAbs, err := filepath.Abs(pythonIndexDB)
	if err != nil {
		return report, fmt.Errorf("resolve Python index DB: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(indexAbs)+"?mode=ro")
	if err != nil {
		return report, fmt.Errorf("open Python index DB: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT operation_id,fingerprint,caller_key,payload_hash,operation,project_id,role,status,result_json,resulting_commit,entry_marker,created_at FROM operations ORDER BY created_at,operation_id`)
	if err != nil {
		return report, fmt.Errorf("read Python operations: %w", err)
	}
	defer rows.Close()
	cutoff := asOf.UTC().Add(-legacyReplayHorizon)
	seenCaller := map[string]bool{}
	operationsDir := filepath.Join(stateAbs, "operations")

	for rows.Next() {
		var operationID, fingerprint, payloadHash, operation, projectID, status, resultJSON, createdAt string
		var callerKey, role, resultingCommit, entryMarker sql.NullString
		if err := rows.Scan(&operationID, &fingerprint, &callerKey, &payloadHash, &operation, &projectID, &role, &status, &resultJSON, &resultingCommit, &entryMarker, &createdAt); err != nil {
			return report, fmt.Errorf("scan Python operation: %w", err)
		}
		report.SourceRows++
		created, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return report, fmt.Errorf("operation %s has invalid created_at: %w", operationID, err)
		}
		if created.Before(cutoff) {
			report.ExpiredRows++
			continue
		}
		if !callerKey.Valid || strings.TrimSpace(callerKey.String) == "" {
			report.UnkeyedActiveRows++
			continue
		}
		if status != "committed" && status != "committed_degraded" {
			return report, fmt.Errorf("active operation %s has unsupported status %q", operationID, status)
		}
		if !prefixedSHA256RE.MatchString(fingerprint) || !prefixedSHA256RE.MatchString(payloadHash) || !prefixedSHA256RE.MatchString(callerKey.String) {
			return report, fmt.Errorf("active operation %s has invalid fingerprint/hash format", operationID)
		}
		if !resultingCommit.Valid || strings.TrimSpace(resultingCommit.String) == "" {
			return report, fmt.Errorf("active operation %s has no resulting commit", operationID)
		}
		ancestor, err := gitIsAncestor(vaultAbs, resultingCommit.String, head)
		if err != nil || !ancestor {
			return report, fmt.Errorf("active operation %s resulting commit is not an ancestor of Vault HEAD", operationID)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
			return report, fmt.Errorf("active operation %s has invalid result JSON: %w", operationID, err)
		}
		if result["operation_id"] != operationID || result["project_id"] != projectID {
			return report, fmt.Errorf("active operation %s result identity mismatch", operationID)
		}
		callerBare := strings.TrimPrefix(callerKey.String, "sha256:")
		if seenCaller[callerBare] {
			return report, fmt.Errorf("multiple active Python operations share caller key %s", callerKey.String)
		}
		result["caller_key_hash"] = callerBare
		record := operationRecord{
			SchemaVersion: 2, OperationID: operationID, Operation: operation, ProjectID: projectID,
			Role: role.String, Status: "committed", ResultingCommit: resultingCommit.String,
			PayloadHash: payloadHash, LegacyFingerprint: fingerprint, CallerKeyHash: callerBare,
			HasEntryMarker: entryMarker.Valid && strings.TrimSpace(entryMarker.String) != "", Result: result,
		}
		path := filepath.Join(operationsDir, operationID+".json")
		if existing, err := os.ReadFile(path); err == nil {
			var current operationRecord
			if json.Unmarshal(existing, &current) != nil || current.OperationID != record.OperationID || current.CallerKeyHash != record.CallerKeyHash || current.LegacyFingerprint != record.LegacyFingerprint || current.ResultingCommit != record.ResultingCommit {
				return report, fmt.Errorf("existing Go operation record %s conflicts with import", operationID)
			}
			report.ExistingRows++
		} else if !os.IsNotExist(err) {
			return report, fmt.Errorf("read existing Go operation record %s: %w", operationID, err)
		} else {
			if err := writeJSONDurable(path, record); err != nil {
				return report, fmt.Errorf("write imported operation %s: %w", operationID, err)
			}
			report.ImportedRows++
		}
		seenCaller[callerBare] = true
	}
	if err := rows.Err(); err != nil {
		return report, fmt.Errorf("iterate Python operations: %w", err)
	}
	report.CallerKeys = len(seenCaller)
	return report, nil
}
