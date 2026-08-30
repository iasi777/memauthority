// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/iasi777/memauthority/internal/checklist"
	"github.com/iasi777/memauthority/internal/vaultread"
	"github.com/iasi777/memauthority/internal/vaultvalidate"
)

type EvidenceInput struct {
	Kind        string `json:"kind"`
	Instance    string `json:"instance"`
	Description string `json:"description"`
}

type SectionUpdateInput struct {
	Heading     string `json:"heading"`
	Replacement string `json:"replacement"`
}

type UpdateHandoffArgs struct {
	ProjectID                string               `json:"project_id"`
	ExpectedResourceRevision string               `json:"expected_resource_revision"`
	SectionUpdates           []SectionUpdateInput `json:"section_updates"`
	Evidence                 []EvidenceInput      `json:"evidence,omitempty"`
	ClientIdempotencyKey     string               `json:"client_idempotency_key,omitempty"`
}

type SectionOperationInput struct {
	Operation string  `json:"operation" jsonschema:"Section operation. Supported H2 operations depend on role; handoff also supports checklist_remove and checklist_set_checked for top-level todo items."`
	Heading   string  `json:"heading" jsonschema:"Target H2 heading. Checklist operations are restricted to 已知问题 / 待办."`
	Content   *string `json:"content,omitempty" jsonschema:"Section body for replace, append, or insert. Do not provide for checklist operations."`
	ItemRef   *string `json:"item_ref,omitempty" jsonschema:"Opaque checklist item_ref returned by memory_read; required only for checklist_remove and checklist_set_checked."`
	Checked   *bool   `json:"checked,omitempty" jsonschema:"Target checked state; required only for checklist_set_checked."`
}

type UpdateSectionsArgs struct {
	ProjectID                string                  `json:"project_id" jsonschema:"Target project id."`
	Role                     string                  `json:"role" jsonschema:"One of handoff, rules, progress, or pitfalls."`
	ExpectedResourceRevision string                  `json:"expected_resource_revision" jsonschema:"SHA-256 revision returned by memory_read; mutation fails on mismatch."`
	Operations               []SectionOperationInput `json:"operations" jsonschema:"Ordered typed section operations. Supported operations depend on role and may replace, append, insert, or delete H2 sections; handoff additionally supports checklist_remove and checklist_set_checked. Checklist operations cannot be mixed with H2 operations in one batch."`
	Evidence                 []EvidenceInput         `json:"evidence,omitempty"`
	ClientIdempotencyKey     string                  `json:"client_idempotency_key,omitempty"`
}

type MarkVerifiedArgs struct {
	ProjectID                string          `json:"project_id"`
	ExpectedResourceRevision string          `json:"expected_resource_revision"`
	VerificationLevel        string          `json:"verification_level" jsonschema:"Verification kind: local verifies source code, configuration, documentation, static results, or other locally inspectable artifacts; runtime verifies the actual running environment, deployment state, or a real business flow."`
	VerifiedScope            []string        `json:"verified_scope"`
	Basis                    []string        `json:"basis"`
	Result                   string          `json:"result"`
	NotVerified              []string        `json:"not_verified,omitempty"`
	Evidence                 []EvidenceInput `json:"evidence,omitempty"`
	ClientIdempotencyKey     string          `json:"client_idempotency_key,omitempty"`
}

type RecordPitfallArgs struct {
	ProjectID            string          `json:"project_id"`
	Title                string          `json:"title"`
	Severity             string          `json:"severity"`
	Tags                 []string        `json:"tags"`
	Content              string          `json:"content"`
	Related              string          `json:"related,omitempty"`
	Evidence             []EvidenceInput `json:"evidence,omitempty"`
	ClientIdempotencyKey string          `json:"client_idempotency_key,omitempty"`
}

type documentMutationSpec struct {
	Operation          string
	ProjectID          string
	Role               string
	ExpectedRevision   string
	ClientKey          string
	Payload            any
	LegacyPayload      any
	EvidenceCount      int
	HasEntryMarker     bool
	MaterializeMissing bool
	DefaultRoleTitle   string
	DuplicateBody      string
	SyncLastVerified   bool
}

type renderFailure struct{ result map[string]any }
type documentRenderer func(raw []byte, payloadHash, effectiveDate string) ([]byte, *renderFailure)

func (s *Service) UpdateHandoff(args UpdateHandoffArgs) map[string]any {
	if len(args.SectionUpdates) == 0 {
		return mutationError("validation_failed", "arguments", "section_updates must not be empty")
	}
	for i, update := range args.SectionUpdates {
		heading := normalizeHeading(update.Heading)
		if heading == "" {
			return mutationError("validation_failed", "arguments", "section update heading is required")
		}
		if heading == "核验记录" {
			return protectedSectionResult(i, "replace", heading, nil)
		}
		if err := validateHandoffReplacement(update.Replacement); err != nil {
			return mutationError("validation_failed", "section_content", err.Error())
		}
	}
	spec := documentMutationSpec{
		Operation: "update_handoff", ProjectID: args.ProjectID, Role: "handoff",
		ExpectedRevision: args.ExpectedResourceRevision, ClientKey: args.ClientIdempotencyKey,
		Payload:       map[string]any{"operation": "update_handoff", "project_id": args.ProjectID, "expected_resource_revision": args.ExpectedResourceRevision, "section_updates": args.SectionUpdates, "evidence": args.Evidence},
		LegacyPayload: map[string]any{"expected_resource_revision": args.ExpectedResourceRevision, "section_updates": p5LegacySectionUpdates(args.SectionUpdates), "evidence": p5LegacyEvidence(args.Evidence)},
		EvidenceCount: len(args.Evidence),
	}
	return s.commitDocumentMutation(spec, func(raw []byte, _, date string) ([]byte, *renderFailure) {
		text := string(raw)
		for _, update := range args.SectionUpdates {
			var err error
			text, err = replaceH2Body(text, cleanHeading(update.Heading), update.Replacement)
			if err != nil {
				return nil, &renderFailure{mutationError("validation_failed", "section", err.Error())}
			}
		}
		return []byte(updateFrontmatterDates(text, date, false)), nil
	})
}

func (s *Service) UpdateSections(args UpdateSectionsArgs) map[string]any {
	if len(args.Operations) == 0 || !validMutationRole(args.Role) {
		return mutationError("validation_failed", "arguments", "role and operations are required")
	}
	if failure := validateChecklistOperationBatch(args); failure != nil {
		return failure
	}
	spec := documentMutationSpec{
		Operation: "update_sections", ProjectID: args.ProjectID, Role: args.Role,
		ExpectedRevision: args.ExpectedResourceRevision, ClientKey: args.ClientIdempotencyKey,
		Payload:       map[string]any{"operation": "update_sections", "project_id": args.ProjectID, "role": args.Role, "expected_resource_revision": args.ExpectedResourceRevision, "operations": args.Operations, "evidence": args.Evidence},
		LegacyPayload: map[string]any{"expected_resource_revision": args.ExpectedResourceRevision, "operations": p5LegacySectionOperations(args.Operations), "evidence": p5LegacyEvidence(args.Evidence)},
		EvidenceCount: len(args.Evidence),
	}
	return s.commitDocumentMutation(spec, func(raw []byte, _, date string) ([]byte, *renderFailure) {
		text := string(raw)
		available := availableSectionNames(text, args.Role)
		if isChecklistOperation(args.Operations[0].Operation) {
			mutations := make([]checklist.Mutation, 0, len(args.Operations))
			for _, op := range args.Operations {
				mutation := checklist.Mutation{ItemRef: strings.TrimSpace(valueOrEmpty(op.ItemRef))}
				if op.Operation == "checklist_remove" {
					mutation.Remove = true
				} else {
					mutation.Checked = op.Checked
				}
				mutations = append(mutations, mutation)
			}
			updated, err := checklist.Apply(raw, args.ExpectedResourceRevision, mutations)
			if err != nil {
				code := "validation_failed"
				message := err.Error()
				failedIndex := 0
				var mutationErr *checklist.MutationError
				if errors.As(err, &mutationErr) && mutationErr.Index >= 0 && mutationErr.Index < len(args.Operations) {
					failedIndex = mutationErr.Index
				}
				if errors.Is(err, checklist.ErrItemNotFound) {
					code = "checklist_item_not_found"
					message = "item_ref does not identify a checklist item in the expected section; re-read the resource"
				}
				return nil, &renderFailure{sectionFailure(code, message, failedIndex, args.Operations[failedIndex], available)}
			}
			return []byte(updateFrontmatterDates(string(updated), date, false)), nil
		}
		for i, op := range args.Operations {
			heading := cleanHeading(op.Heading)
			if args.Role == "handoff" && normalizeHeading(op.Heading) == "核验记录" {
				return nil, &renderFailure{protectedSectionResult(i, op.Operation, "核验记录", available)}
			}
			if heading == "" || !operationAllowed(args.Role, op.Operation) {
				return nil, &renderFailure{sectionFailure("validation_failed", "invalid section operation", i, op, available)}
			}
			if op.Operation != "delete" {
				if op.Content == nil {
					return nil, &renderFailure{sectionFailure("validation_failed", "section content is required", i, op, available)}
				}
				if err := validateSectionBody(*op.Content); err != nil {
					return nil, &renderFailure{sectionFailure("validation_failed", err.Error(), i, op, available)}
				}
			}
			var err error
			switch op.Operation {
			case "replace":
				text, err = replaceH2Body(text, heading, valueOrEmpty(op.Content))
			case "append":
				text, err = appendH2Body(text, heading, valueOrEmpty(op.Content))
			case "delete":
				text, err = deleteH2Section(text, heading)
			case "insert":
				text, err = insertH2Section(text, normalizeHeading(op.Heading), valueOrEmpty(op.Content))
			}
			if err != nil {
				return nil, &renderFailure{sectionFailure("validation_failed", err.Error(), i, op, available)}
			}
		}
		return []byte(updateFrontmatterDates(text, date, false)), nil
	})
}

func (s *Service) MarkVerified(args MarkVerifiedArgs) map[string]any {
	if strings.TrimSpace(args.ProjectID) == "" || args.ExpectedResourceRevision == "" || len(args.VerifiedScope) == 0 || len(args.Basis) == 0 || strings.TrimSpace(args.Result) == "" {
		return mutationError("validation_failed", "arguments", "verification arguments are incomplete")
	}
	if args.VerificationLevel != "local" && args.VerificationLevel != "runtime" {
		return mutationError("validation_failed", "verification_level", "verification_level must be one of: local, runtime")
	}
	spec := documentMutationSpec{
		Operation: "mark_verified", ProjectID: args.ProjectID, Role: "handoff",
		ExpectedRevision: args.ExpectedResourceRevision, ClientKey: args.ClientIdempotencyKey,
		Payload:       map[string]any{"operation": "mark_verified", "project_id": args.ProjectID, "expected_resource_revision": args.ExpectedResourceRevision, "verification_level": args.VerificationLevel, "verified_scope": args.VerifiedScope, "basis": args.Basis, "result": args.Result, "not_verified": args.NotVerified, "evidence": args.Evidence},
		LegacyPayload: map[string]any{"expected_resource_revision": args.ExpectedResourceRevision, "verification_level": args.VerificationLevel, "verified_scope": p5LegacyStrings(args.VerifiedScope), "basis": p5LegacyStrings(args.Basis), "result": p5LegacyCleanText(args.Result), "not_verified": p5LegacyStrings(args.NotVerified), "evidence": p5LegacyEvidence(args.Evidence)},
		EvidenceCount: len(args.Evidence), SyncLastVerified: true,
	}
	return s.commitDocumentMutation(spec, func(raw []byte, _, date string) ([]byte, *renderFailure) {
		level := "本地核验"
		if args.VerificationLevel == "runtime" {
			level = "运行时核验"
		}
		body := "- 核验于: " + date + "\n" +
			"- 核验级别: " + level + "\n" +
			"- 核验范围: " + strings.Join(args.VerifiedScope, "、") + "\n" +
			"- 核验依据: " + strings.Join(args.Basis, "、") + "\n" +
			"- 结果: " + strings.TrimSpace(args.Result) + "\n"
		if len(args.NotVerified) > 0 {
			body += "- 未核验: " + strings.Join(args.NotVerified, "、") + "\n"
		}
		text := string(raw)
		var updated string
		var err error
		if hasH2(text, "核验记录") {
			updated, err = replaceH2Body(text, "核验记录", body)
		} else {
			updated, err = insertH2AfterRoot(text, "核验记录", body)
		}
		if err != nil {
			return nil, &renderFailure{mutationError("validation_failed", "verification", err.Error())}
		}
		return []byte(updateFrontmatterDates(updated, date, true)), nil
	})
}

func (s *Service) RecordPitfall(args RecordPitfallArgs) map[string]any {
	if strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Title) == "" || strings.TrimSpace(args.Content) == "" {
		return mutationError("validation_failed", "arguments", "project_id, title, and content are required")
	}
	switch args.Severity {
	case "low", "medium", "high", "critical":
	default:
		return mutationError("validation_failed", "severity", "unsupported pitfall severity")
	}
	if strings.ContainsAny(args.Title, "\r\n") || containsMarkdownHeading(args.Content) {
		return mutationError("validation_failed", "content", "pitfall title/content must not supply Markdown headings")
	}
	if containsHighConfidenceSecret(args.Content) {
		return mutationError("validation_failed", "content_secret", "pitfall content contains high-confidence secret material")
	}
	spec := documentMutationSpec{
		Operation: "record_pitfall", ProjectID: args.ProjectID, Role: "pitfalls", ClientKey: args.ClientIdempotencyKey,
		Payload:       map[string]any{"operation": "record_pitfall", "project_id": args.ProjectID, "title": args.Title, "severity": args.Severity, "tags": args.Tags, "content": args.Content, "related": args.Related, "evidence": args.Evidence},
		LegacyPayload: map[string]any{"title": p5LegacyCleanText(args.Title), "severity": args.Severity, "tags": p5LegacyStrings(args.Tags), "content": p5LegacyCleanText(args.Content), "related": p5LegacyRelated(args.Related), "evidence": p5LegacyEvidence(args.Evidence)},
		EvidenceCount: len(args.Evidence), HasEntryMarker: true,
		MaterializeMissing: true, DefaultRoleTitle: "踩坑", DuplicateBody: args.Content,
	}
	return s.commitDocumentMutation(spec, func(raw []byte, payloadHash, date string) ([]byte, *renderFailure) {
		text := string(raw)
		if _, err := roleRootH1InsertOffset(text, "踩坑"); err != nil {
			return nil, &renderFailure{mutationError("validation_failed", "pitfalls", err.Error())}
		}
		metadata := fmt.Sprintf("`severity: %s` · `tags: %s`", args.Severity, strings.Join(args.Tags, ", "))
		if strings.TrimSpace(args.Related) != "" {
			metadata += " · `related: " + strings.TrimSpace(args.Related) + "`"
		}
		body := metadata + "\n\n" + strings.TrimSpace(args.Content) + "\n\n<!-- v-memory-entry:" + strings.TrimPrefix(payloadHash, "sha256:") + " -->"
		updated, err := insertH2AfterRoot(text, "["+strings.TrimSpace(args.Title)+"] - "+date, body)
		if err != nil {
			return nil, &renderFailure{mutationError("validation_failed", "pitfalls", err.Error())}
		}
		return []byte(updateFrontmatterDates(updated, date, false)), nil
	})
}

func (s *Service) commitDocumentMutation(spec documentMutationSpec, render documentRenderer) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return preflight
	}
	if invalidProjectIDPath(spec.ProjectID) {
		return mutationError("validation_failed", "project_id_path", "project_id is invalid")
	}
	if payloadContainsHighConfidenceSecret(spec.Payload) {
		return mutationError("validation_failed", "content_secret", "durable mutation content contains high-confidence secret material")
	}
	payloadHash, err := hashPayload(spec.Payload)
	if err != nil {
		return mutationError("mutation_failed", "payload", err.Error())
	}
	callerKeyHash := ""
	if spec.ClientKey != "" {
		callerKeyHash = hashText(spec.ClientKey)
	}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, spec.ClientKey, payloadHash, spec.ProjectID, spec.Role, spec.Operation, spec.LegacyPayload); handled {
		return result
	}
	if callerKeyHash != "" && spec.HasEntryMarker {
		if existing, err := s.findRepresentedEntry(payloadHash, spec.ProjectID, spec.Role); err == nil && existing != nil {
			return replayResult(existing.Result)
		} else if err != nil {
			return mutationError("mutation_failed", "idempotency_entry", err.Error())
		}
	}
	clean, dirty, err := gitWorktreeState(s.root)
	if err != nil {
		return mutationError("mutation_failed", "workspace", err.Error())
	}
	if !clean {
		return map[string]any{"status": "error", "code": "read_only", "rule": "dirty_workspace", "message": "Authority worktree is dirty", "dirty_paths": dirty, "audit_state": "recorded"}
	}
	effectiveDate := s.todayLocal()
	reader, err := vaultread.Open(s.root)
	if err != nil {
		return mutationError("validation_failed", "vault", err.Error())
	}
	if !readerHasProject(reader, spec.ProjectID) {
		_ = reader.Close()
		return mutationError("validation_failed", "project_id", "unknown project")
	}
	previousCommit := reader.SourceCommit()
	rolePath, roleErr := reader.ResolveRoleFile(spec.ProjectID, spec.Role)
	_ = reader.Close()
	missing := roleErr != nil
	if missing {
		if !spec.MaterializeMissing {
			return mutationError("validation_failed", spec.Role, roleErr.Error())
		}
		rolePath, err = rolePathAtWorktree(s.root, spec.ProjectID, spec.Role)
		if err != nil {
			return mutationError("validation_failed", spec.Role, err.Error())
		}
	}
	var raw []byte
	previousRevision := ""
	if missing {
		raw = renderDefaultRoleFile(spec.DefaultRoleTitle, spec.ProjectID, effectiveDate)
		if len(raw) == 0 {
			return mutationError("validation_failed", spec.Role, "missing role cannot be materialized")
		}
	} else {
		rel, relErr := filepath.Rel(s.root, rolePath)
		if relErr != nil {
			return mutationError("mutation_failed", spec.Role, fmt.Sprintf("resolve resource path: %v", relErr))
		}
		raw, err = readAuthorityAtCommit(s.root, previousCommit, rel)
		if err != nil {
			return mutationError("mutation_failed", spec.Role, fmt.Sprintf("read resource at Authority commit: %v", err))
		}
		if !utf8.Valid(raw) {
			return mutationError("validation_failed", spec.Role, "resource is not valid UTF-8")
		}
		previousRevision = revision(raw)
		if spec.DuplicateBody != "" && canonicalBodyPresent(string(raw), spec.DuplicateBody) {
			return mutationError("validation_failed", "duplicate_entry", "equivalent append already exists in the current resource")
		}
	}
	raw = canonicalManagedText(raw)
	if spec.ExpectedRevision != "" && spec.ExpectedRevision != previousRevision {
		result := mutationError("conflict", "", "resource revision does not match current content")
		result["current_revision"] = previousRevision
		return result
	}
	candidateContent, failure := render(raw, payloadHash, effectiveDate)
	if failure != nil {
		return failure.result
	}
	if !utf8.Valid(candidateContent) {
		return mutationError("validation_failed", spec.Role, "candidate resource is not valid UTF-8")
	}
	candidateContent = canonicalManagedText(candidateContent)
	newRevision := revision(candidateContent)
	operationID, err := randomOperationID()
	if err != nil {
		return mutationError("mutation_failed", "operation_id", err.Error())
	}
	relativePath, err := filepath.Rel(s.root, rolePath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return mutationError("validation_failed", "path", "resource escapes Vault root")
	}
	relativePath = filepath.ToSlash(relativePath)
	candidateRoot := filepath.Join(s.workRoot, "candidate-"+operationID)
	defer os.RemoveAll(candidateRoot)
	if err := copyAuthorityTree(s.root, candidateRoot); err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
	}
	changes, err := s.normalRoleMutationChanges(spec.ProjectID, relativePath, candidateContent, effectiveDate, spec.SyncLastVerified)
	if err != nil {
		return mutationError("validation_failed", "index", err.Error())
	}
	if err := applyCandidateChanges(candidateRoot, changes); err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
	}
	if err := vaultvalidate.ValidateAuthorityRoot(candidateRoot); err != nil {
		return mutationError("validation_failed", "vault", err.Error())
	}
	candidateCommit, err := s.buildCandidateCommitChanges(previousCommit, changes, operationID, spec.Operation, spec.ProjectID)
	if err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
	}
	plannedResult := committedDocumentResult(spec, operationID, callerKeyHash, previousCommit, candidateCommit, previousRevision, newRevision, s.writeSource)
	journal := journalRecord{SchemaVersion: 2, OperationID: operationID, Operation: spec.Operation, ProjectID: spec.ProjectID, Role: spec.Role, State: "prepared", PreviousCommit: previousCommit, CandidateCommit: candidateCommit, RelativePath: relativePath, PayloadHash: payloadHash, CallerKeyHash: callerKeyHash, HasEntryMarker: spec.HasEntryMarker, Result: cloneMap(plannedResult)}
	journalPath := filepath.Join(s.journalDir, operationID+".json")
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	s.maybeCrash("journal_persisted_before_ref")
	if err := s.maybeInjectExternalRefMovement(); err != nil {
		journal.State = "aborted"
		_ = writeJSONDurable(journalPath, journal)
		return mutationError("mutation_failed", "test_fault", err.Error())
	}
	if err := gitCASHead(s.root, previousCommit, candidateCommit); err != nil {
		journal.State = "aborted"
		_ = writeJSONDurable(journalPath, journal)
		return mutationError("conflict", "git_cas", "Authority Git HEAD moved before compare-and-swap")
	}
	journal.State = "ref_updated"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	s.maybeCrash("ref_updated_before_materialize")
	if err := materializeCandidate(s.root, candidateCommit, changes); err != nil {
		return mutationError("mutation_failed", "materialize", err.Error())
	}
	journal.State = "materialized"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	s.maybeCrash("materialized_before_index")
	if s.crashFaultPhase == "index_failure_before_rollback" {
		journal.State = "index_failed"
		if err := writeJSONDurable(journalPath, journal); err != nil {
			return mutationError("mutation_failed", "journal", err.Error())
		}
		s.maybeCrash("index_failure_before_rollback")
	}
	indexReader, err := vaultread.OpenWithOptions(s.root, vaultread.OpenOptions{ProjectionPath: s.projection, ForceRebuild: true})
	if err != nil {
		_ = s.rollbackAfterIndexFailure(previousCommit, candidateCommit, journalPath, &journal)
		return mutationError("mutation_failed", "index", err.Error())
	}
	indexedHead := indexReader.IndexHead()
	_ = indexReader.Close()
	if indexedHead != candidateCommit {
		_ = s.rollbackAfterIndexFailure(previousCommit, candidateCommit, journalPath, &journal)
		return mutationError("mutation_failed", "index", "projection did not align with candidate commit")
	}
	s.indexHead = indexedHead
	journal.State = "indexed"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	result := plannedResult
	record := operationRecord{SchemaVersion: 2, OperationID: operationID, Operation: spec.Operation, ProjectID: spec.ProjectID, Role: spec.Role, Status: "committed", ResultingCommit: candidateCommit, PayloadHash: payloadHash, CallerKeyHash: callerKeyHash, HasEntryMarker: spec.HasEntryMarker, Result: cloneMap(result)}
	if err := writeJSONDurable(filepath.Join(s.operationsDir, operationID+".json"), record); err != nil {
		return mutationError("mutation_failed", "operation_record", err.Error())
	}
	journal.State = "finalized"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	s.maybeCrash("finalized_before_result_delivery")
	return result
}

func committedDocumentResult(spec documentMutationSpec, operationID, callerKeyHash, previousCommit, newCommit, previousRevision, newRevision, source string) map[string]any {
	var previousRevisionValue any
	if previousRevision != "" {
		previousRevisionValue = previousRevision
	}
	return map[string]any{
		"status": "committed", "code": nil, "message": nil, "rule": nil,
		"project_id": spec.ProjectID, "resource_uri": fmt.Sprintf("memory://projects/%s/%s", spec.ProjectID, spec.Role),
		"previous_commit": previousCommit, "new_commit": newCommit, "previous_revision": previousRevisionValue, "new_revision": newRevision, "content_hash": newRevision,
		"operation_id": operationID, "caller_key_hash": callerKeyHash, "incremental_validation": "pass", "index_state": "current", "backup_state": "disabled",
		"idempotent_replay": false, "source": source, "evidence_count": spec.EvidenceCount, "audit_state": "recorded",
		"all_rolled_back": nil, "archive_reason": nil, "archived_at": nil, "available_sections": nil, "current_revision": nil, "failed_operation": nil, "failed_operation_index": nil, "runtime_status": nil,
	}
}

func (s *Service) buildCandidateCommitFor(previousCommit, relativePath string, content []byte, operationID, operation, projectID string) (string, error) {
	blob, err := gitInput(s.root, nil, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("write candidate blob: %w", err)
	}
	indexPath := filepath.Join(s.workRoot, "git-index-"+operationID)
	defer os.Remove(indexPath)
	env := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if _, err := gitInput(s.root, env, nil, "read-tree", previousCommit); err != nil {
		return "", fmt.Errorf("read candidate tree: %w", err)
	}
	if _, err := gitInput(s.root, env, nil, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+relativePath); err != nil {
		return "", fmt.Errorf("update candidate index: %w", err)
	}
	tree, err := gitInput(s.root, env, nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write candidate tree: %w", err)
	}
	message := fmt.Sprintf("v-memory: %s %s\n", strings.ReplaceAll(operation, "_", " "), projectID)
	commit, err := gitInput(s.root, serviceGitIdentityEnv(nil), []byte(message), "commit-tree", tree, "-p", previousCommit)
	if err != nil {
		return "", fmt.Errorf("create candidate commit: %w", err)
	}
	return commit, nil
}

type h2Section struct {
	heading   string
	start     int
	bodyStart int
	end       int
}

func sectionsH2(text string) []h2Section {
	lines := strings.SplitAfter(text, "\n")
	sections := make([]h2Section, 0)
	offset := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
			if len(sections) > 0 {
				sections[len(sections)-1].end = offset
			}
			sections = append(sections, h2Section{heading: strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), start: offset, bodyStart: offset + len(line), end: len(text)})
		}
		offset += len(line)
	}
	return sections
}

func findH2(text, heading string) (h2Section, bool) {
	target := cleanHeading(heading)
	sections := sectionsH2(text)

	// Match the literal H2 title first. A title may itself contain " / ";
	// treating that delimiter as a path before exact matching loses valid titles.
	for _, section := range sections {
		if section.heading == target {
			return section, true
		}
	}

	// Python reference also accepts the real heading path (H1 / H2).
	if root := rootH1(text); root != "" {
		for _, section := range sections {
			if root+" / "+section.heading == target {
				return section, true
			}
		}
	}

	// Preserve the pre-correction leaf-path compatibility for callers that
	// already send an available-section style path. This fallback runs only
	// after exact-title and real-path matching, so literal slash titles win.
	leaf := normalizeHeading(target)
	if leaf != target {
		for _, section := range sections {
			if section.heading == leaf {
				return section, true
			}
		}
	}
	return h2Section{}, false
}

func rootH1(text string) string {
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func hasH2(text, heading string) bool { _, ok := findH2(text, heading); return ok }

func cleanHeading(heading string) string {
	heading = strings.TrimSpace(heading)
	return strings.TrimSpace(strings.TrimPrefix(heading, "## "))
}

func normalizeHeading(heading string) string {
	heading = strings.TrimSpace(heading)
	if i := strings.LastIndex(heading, " / "); i >= 0 {
		heading = strings.TrimSpace(heading[i+3:])
	}
	return strings.TrimSpace(strings.TrimPrefix(heading, "## "))
}

func normalizeBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "\n"
	}
	return body + "\n\n"
}

func replaceH2Body(text, heading, body string) (string, error) {
	section, ok := findH2(text, heading)
	if !ok {
		return "", fmt.Errorf("section %q was not found", heading)
	}
	return text[:section.bodyStart] + normalizeBody(body) + text[section.end:], nil
}

func appendH2Body(text, heading, body string) (string, error) {
	section, ok := findH2(text, heading)
	if !ok {
		return "", fmt.Errorf("section %q was not found", heading)
	}
	existing := strings.TrimSpace(text[section.bodyStart:section.end])
	addition := strings.TrimSpace(body)
	if existing != "" && addition != "" {
		existing += "\n\n"
	}
	return text[:section.bodyStart] + normalizeBody(existing+addition) + text[section.end:], nil
}

func deleteH2Section(text, heading string) (string, error) {
	section, ok := findH2(text, heading)
	if !ok {
		return "", fmt.Errorf("section %q was not found", heading)
	}
	return text[:section.start] + text[section.end:], nil
}

func insertH2Section(text, heading, body string) (string, error) {
	if hasH2(text, heading) {
		return "", fmt.Errorf("section %q already exists", heading)
	}
	return insertH2AfterRoot(text, heading, body)
}

func insertH2AfterRoot(text, heading, body string) (string, error) {
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		offset += len(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			entry := "\n## " + strings.TrimSpace(heading) + "\n" + normalizeBody(body)
			return text[:offset] + entry + text[offset:], nil
		}
	}
	return "", fmt.Errorf("resource is missing its root H1 heading")
}

func updateFrontmatterDates(text, date string, verified bool) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	endRel := strings.Index(text[4:], "\n---\n")
	if endRel < 0 {
		return text
	}
	end := 4 + endRel + len("\n---\n")
	front := replaceFrontmatterDate(text[:end], "最后更新", date)
	if verified {
		front = replaceFrontmatterDate(front, "最后核验", date)
	}
	return front + text[end:]
}

func validateHandoffReplacement(body string) error {
	if !utf8.ValidString(body) || containsMarkdownHeading(body) {
		return fmt.Errorf("handoff replacement must be UTF-8 body text without Markdown headings")
	}
	return nil
}

func validateSectionBody(body string) error {
	if containsHighConfidenceSecret(body) {
		return fmt.Errorf("section content contains high-confidence secret material")
	}
	if !utf8.ValidString(body) {
		return fmt.Errorf("section content must be UTF-8")
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "##### ") || strings.HasPrefix(trimmed, "###### ") {
			return fmt.Errorf("section content may contain only H3/H4 subheadings")
		}
	}
	return nil
}

func containsMarkdownHeading(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return true
		}
	}
	return false
}

func validMutationRole(role string) bool {
	switch role {
	case "handoff", "rules", "progress", "pitfalls":
		return true
	default:
		return false
	}
}

func operationAllowed(role, operation string) bool {
	switch role {
	case "handoff":
		return operation == "replace" || operation == "append" || isChecklistOperation(operation)
	case "progress", "pitfalls":
		return operation == "replace" || operation == "delete"
	case "rules":
		return operation == "replace" || operation == "append" || operation == "insert" || operation == "delete"
	default:
		return false
	}
}

func isChecklistOperation(operation string) bool {
	return operation == "checklist_remove" || operation == "checklist_set_checked"
}

func validateChecklistOperationBatch(args UpdateSectionsArgs) map[string]any {
	hasChecklist := false
	hasSection := false
	for i, op := range args.Operations {
		if isChecklistOperation(op.Operation) {
			hasChecklist = true
			if args.Role != "handoff" {
				return sectionFailure("validation_failed", "checklist operations are supported only for handoff", i, op, nil)
			}
			if cleanHeading(op.Heading) != checklist.TodoHeading {
				return sectionFailure("validation_failed", "checklist operations are restricted to 已知问题 / 待办", i, op, nil)
			}
			if op.Content != nil {
				return sectionFailure("validation_failed", "content must not be provided for checklist operations", i, op, nil)
			}
			if op.ItemRef == nil || strings.TrimSpace(*op.ItemRef) == "" {
				return sectionFailure("validation_failed", "item_ref is required for checklist operations", i, op, nil)
			}
			if op.Operation == "checklist_remove" && op.Checked != nil {
				return sectionFailure("validation_failed", "checked must not be provided for checklist_remove", i, op, nil)
			}
			if op.Operation == "checklist_set_checked" && op.Checked == nil {
				return sectionFailure("validation_failed", "checked is required for checklist_set_checked", i, op, nil)
			}
			continue
		}

		hasSection = true
		if op.ItemRef != nil || op.Checked != nil {
			return sectionFailure("validation_failed", "item_ref and checked are only valid for checklist operations", i, op, nil)
		}
	}
	if hasChecklist && hasSection {
		return mutationError("validation_failed", "operations", "checklist operations cannot be mixed with H2 section operations in one batch")
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sectionFailure(code, message string, index int, operation SectionOperationInput, available []string) map[string]any {
	result := mutationError(code, "", message)
	result["failed_operation_index"] = index
	result["failed_operation"] = map[string]any{"operation": operation.Operation, "heading": normalizeHeading(operation.Heading)}
	result["all_rolled_back"] = true
	result["available_sections"] = available
	return result
}

func protectedSectionResult(index int, operation, heading string, available []string) map[string]any {
	result := mutationError("section_protected", "", "核验记录 is writable only through memory_mark_verified")
	result["failed_operation_index"] = index
	result["failed_operation"] = map[string]any{"operation": operation, "heading": heading}
	result["all_rolled_back"] = true
	result["available_sections"] = available
	return result
}

func availableSectionNames(text, role string) []string {
	prefix := map[string]string{"handoff": "交接", "rules": "规范", "progress": "进度", "pitfalls": "踩坑"}[role]
	out := make([]string, 0)
	for _, section := range sectionsH2(text) {
		out = append(out, prefix+" / "+section.heading)
	}
	return out
}
