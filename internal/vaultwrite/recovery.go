// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type recoveryState struct {
	Blocked         bool
	RecoveredCount  int
	DiscardedCount  int
	RolledBackCount int
	IndexBlocked    bool
	IndexReason     string
}

func (s *Service) RecoverySummary() map[string]any {
	return map[string]any{
		"blocked":           s.startupRecovery.Blocked,
		"recovered_count":   s.startupRecovery.RecoveredCount,
		"discarded_count":   s.startupRecovery.DiscardedCount,
		"rolled_back_count": s.startupRecovery.RolledBackCount,
		"index_blocked":     s.startupRecovery.IndexBlocked,
		"index_reason":      s.startupRecovery.IndexReason,
	}
}

func (s *Service) maybeCrash(phase string) {
	if s.crashHook != nil && s.crashFaultPhase == phase {
		s.crashHook(phase)
	}
}

func (s *Service) recoverStartup() (bool, error) {
	entries, err := os.ReadDir(s.journalDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rebuild := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.journalDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return rebuild, err
		}
		var journal journalRecord
		if err := json.Unmarshal(raw, &journal); err != nil {
			return rebuild, fmt.Errorf("read recovery journal %s: %w", entry.Name(), err)
		}
		switch journal.State {
		case "finalized", "rolled_back", "aborted", "discarded", "recovered":
			continue
		}
		head, err := gitHead(s.root)
		if err != nil {
			return rebuild, err
		}
		candidateAuthoritative := head == journal.CandidateCommit
		if !candidateAuthoritative && journal.CandidateCommit != "" {
			candidateAuthoritative, _ = gitIsAncestor(s.root, journal.CandidateCommit, head)
		}
		if journal.State == "index_failed" {
			if candidateAuthoritative || head == journal.CandidateCommit {
				if head == journal.CandidateCommit {
					if err := gitCASHead(s.root, journal.CandidateCommit, journal.PreviousCommit); err != nil {
						return rebuild, fmt.Errorf("complete interrupted rollback CAS: %w", err)
					}
				} else {
					return rebuild, fmt.Errorf("cannot roll back interrupted index failure after later authoritative commits")
				}
				if err := gitResetHard(s.root, journal.PreviousCommit); err != nil {
					return rebuild, fmt.Errorf("materialize interrupted rollback: %w", err)
				}
				journal.State = "rolled_back"
				if err := writeJSONDurable(path, journal); err != nil {
					return rebuild, err
				}
				s.startupRecovery.RolledBackCount++
				rebuild = true
				continue
			}
			journal.State = "rolled_back"
			if err := writeJSONDurable(path, journal); err != nil {
				return rebuild, err
			}
			s.startupRecovery.RolledBackCount++
			rebuild = true
			continue
		}
		if candidateAuthoritative {
			if err := gitResetHard(s.root, head); err != nil {
				return rebuild, fmt.Errorf("materialize authoritative recovered HEAD: %w", err)
			}
			if err := s.ensureRecoveredOperation(journal); err != nil {
				return rebuild, err
			}
			journal.State = "recovered"
			if err := writeJSONDurable(path, journal); err != nil {
				return rebuild, err
			}
			s.startupRecovery.RecoveredCount++
			rebuild = true
			continue
		}
		previousStillAuthority := head == journal.PreviousCommit
		if !previousStillAuthority && journal.PreviousCommit != "" {
			previousStillAuthority, _ = gitIsAncestor(s.root, journal.PreviousCommit, head)
		}
		if previousStillAuthority {
			journal.State = "discarded"
			if err := writeJSONDurable(path, journal); err != nil {
				return rebuild, err
			}
			s.startupRecovery.DiscardedCount++
			rebuild = true
			continue
		}
		return rebuild, fmt.Errorf("journal %s cannot be reconciled with authoritative HEAD %s", journal.OperationID, head)
	}
	return rebuild, nil
}

func (s *Service) ensureRecoveredOperation(journal journalRecord) error {
	if journal.Result == nil {
		return fmt.Errorf("journal %s lacks durable result evidence required for recovery", journal.OperationID)
	}
	path := filepath.Join(s.operationsDir, journal.OperationID+".json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	record := operationRecord{
		SchemaVersion:   2,
		OperationID:     journal.OperationID,
		Operation:       journal.Operation,
		ProjectID:       journal.ProjectID,
		Role:            journal.Role,
		Status:          "committed",
		ResultingCommit: journal.CandidateCommit,
		PayloadHash:     journal.PayloadHash,
		CallerKeyHash:   journal.CallerKeyHash,
		HasEntryMarker:  journal.HasEntryMarker,
		Result:          cloneMap(journal.Result),
	}
	return writeJSONDurable(path, record)
}

func gitIsAncestor(root, ancestor, descendant string) (bool, error) {
	_, err := gitInput(root, nil, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	return false, nil
}

func (s *Service) findRepresentedEntry(payloadHash, projectID, role string) (*operationRecord, error) {
	records, err := s.operationRecords()
	if err != nil {
		return nil, err
	}
	marker := "<!-- v-memory-entry:" + strings.TrimPrefix(payloadHash, "sha256:") + " -->"
	for i := range records {
		if records[i].PayloadHash != payloadHash || !records[i].HasEntryMarker || records[i].ProjectID != projectID || records[i].Role != role {
			continue
		}
		readerPath, err := rolePathAtWorktree(s.root, projectID, role)
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(readerPath)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(raw), marker) {
			return &records[i], nil
		}
	}
	return nil, nil
}

func rolePathAtWorktree(root, projectID, role string) (string, error) {
	filename := map[string]string{"handoff": "交接.md", "rules": "规范.md", "progress": "进度.md", "pitfalls": "踩坑.md"}[role]
	if filename == "" || invalidProjectIDPath(projectID) {
		return "", fmt.Errorf("invalid role path")
	}
	return filepath.Join(root, projectID, filename), nil
}
