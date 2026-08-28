// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iasi777/v-memory/internal/gitview"
	"github.com/iasi777/v-memory/internal/textcanon"
	"github.com/iasi777/v-memory/internal/vaultread"
	"github.com/iasi777/v-memory/internal/vaultvalidate"
)

const progressFilename = "进度.md"

type Config struct {
	WorkRoot        string
	WriteEnabled    bool
	WriteSource     string
	RequirePrimary  bool
	PrimaryEpoch    int
	LocalNodeID     string
	TestFault       string
	CrashFaultPhase string
	CrashHook       func(string)
	Now             func() time.Time
}

type AppendProgressArgs struct {
	ProjectID            string `json:"project_id"`
	Summary              string `json:"summary"`
	Content              string `json:"content" jsonschema:"Progress entry body. A single leading H1/H2 title is ignored because V-Memory owns the entry heading; H3/H4 subheadings are allowed, but H1/H2 headings inside the body are rejected."`
	ClientIdempotencyKey string `json:"client_idempotency_key,omitempty"`
}

type Service struct {
	root            string
	workRoot        string
	projection      string
	journalDir      string
	operationsDir   string
	writeEnabled    bool
	writeSource     string
	requirePrimary  bool
	primaryEpoch    int
	localNodeID     string
	testFault       string
	faultInjected   bool
	indexHead       string
	crashFaultPhase string
	crashHook       func(string)
	startupRecovery recoveryState
	legacyIndex     bool
	now             func() time.Time
}

type journalRecord struct {
	SchemaVersion   int            `json:"schema_version"`
	OperationID     string         `json:"operation_id"`
	Operation       string         `json:"operation"`
	ProjectID       string         `json:"project_id"`
	Role            string         `json:"role"`
	State           string         `json:"state"`
	PreviousCommit  string         `json:"previous_commit"`
	CandidateCommit string         `json:"candidate_commit"`
	RelativePath    string         `json:"relative_path"`
	PayloadHash     string         `json:"payload_hash"`
	CallerKeyHash   string         `json:"caller_key_hash,omitempty"`
	HasEntryMarker  bool           `json:"has_entry_marker,omitempty"`
	Result          map[string]any `json:"result,omitempty"`
}

type operationRecord struct {
	SchemaVersion     int            `json:"schema_version"`
	OperationID       string         `json:"operation_id"`
	Operation         string         `json:"operation"`
	ProjectID         string         `json:"project_id"`
	Role              string         `json:"role"`
	Status            string         `json:"status"`
	ResultingCommit   string         `json:"resulting_commit"`
	PayloadHash       string         `json:"payload_hash"`
	LegacyFingerprint string         `json:"legacy_fingerprint,omitempty"`
	CallerKeyHash     string         `json:"caller_key_hash,omitempty"`
	HasEntryMarker    bool           `json:"has_entry_marker"`
	Result            map[string]any `json:"result"`
}

func New(root string, config Config) (*Service, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root symlinks: %w", err)
	}
	workRoot := config.WorkRoot
	if strings.TrimSpace(workRoot) == "" {
		return nil, fmt.Errorf("work root is required")
	}
	workAbs, err := filepath.Abs(workRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve work root: %w", err)
	}
	if err := os.MkdirAll(workAbs, 0o755); err != nil {
		return nil, fmt.Errorf("create work root: %w", err)
	}
	source := strings.TrimSpace(config.WriteSource)
	if source == "" {
		source = "unknown"
	}
	s := &Service{
		root:            resolved,
		workRoot:        workAbs,
		projection:      filepath.Join(workAbs, "read-index.sqlite"),
		journalDir:      filepath.Join(workAbs, "transactions"),
		operationsDir:   filepath.Join(workAbs, "operations"),
		writeEnabled:    config.WriteEnabled,
		writeSource:     source,
		requirePrimary:  config.RequirePrimary,
		primaryEpoch:    config.PrimaryEpoch,
		localNodeID:     strings.TrimSpace(config.LocalNodeID),
		testFault:       config.TestFault,
		crashFaultPhase: config.CrashFaultPhase,
		crashHook:       config.CrashHook,
		startupRecovery: recoveryState{},
		now:             config.Now,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.requirePrimary && s.primaryEpoch < 1 {
		return nil, fmt.Errorf("primary epoch must be positive when primary fencing is required")
	}
	rebuildProjection, err := s.recoverStartup()
	if err != nil {
		return nil, fmt.Errorf("startup transaction recovery: %w", err)
	}
	reader, err := vaultread.OpenWithOptions(resolved, vaultread.OpenOptions{ProjectionPath: s.projection, ForceRebuild: rebuildProjection})
	if err != nil {
		if isLegacyLifecycleIndex(resolved) {
			head, headErr := gitHead(resolved)
			if headErr != nil {
				return nil, fmt.Errorf("initialize legacy write head: %w", headErr)
			}
			s.indexHead = head
			s.legacyIndex = true
			s.startupRecovery.IndexReason = "legacy"
			return s, nil
		}
		return nil, fmt.Errorf("initialize write projection: %w", err)
	}
	s.indexHead = reader.IndexHead()
	_ = reader.Close()
	s.startupRecovery.IndexReason = "current"
	return s, nil
}

func (s *Service) AppendProgress(args AppendProgressArgs) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return preflight
	}
	if invalidProjectIDPath(args.ProjectID) {
		return mutationError("validation_failed", "project_id_path", "project_id is invalid")
	}
	args.Content = normalizeProgressContent(args.Content)
	if containsProgressStructuralHeading(args.Content) {
		return mutationError("validation_failed", "content_heading", "append content may use H3/H4 subheadings, but must not contain H1/H2 headings inside the entry")
	}
	if containsHighConfidenceSecret(args.Content) {
		return mutationError("validation_failed", "content_secret", "append content contains high-confidence secret material")
	}
	if err := validateAppendArgs(args); err != nil {
		return mutationError("validation_failed", "arguments", err.Error())
	}

	payload := map[string]any{
		"operation":  "append_progress",
		"project_id": args.ProjectID,
		"summary":    args.Summary,
		"content":    args.Content,
	}
	if payloadContainsHighConfidenceSecret(payload) {
		return mutationError("validation_failed", "content_secret", "append content contains high-confidence secret material")
	}
	payloadHash, err := hashPayload(payload)
	if err != nil {
		return mutationError("mutation_failed", "", err.Error())
	}
	callerKeyHash := ""
	if args.ClientIdempotencyKey != "" {
		callerKeyHash = hashText(args.ClientIdempotencyKey)
	}
	legacyPayload := map[string]any{"summary": p5LegacyCleanText(args.Summary), "content": p5LegacyCleanText(args.Content), "evidence": []EvidenceInput{}}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, args.ClientIdempotencyKey, payloadHash, args.ProjectID, "progress", "append_progress", legacyPayload); handled {
		return result
	}
	if callerKeyHash != "" {
		if existing, err := s.findRepresentedEntry(payloadHash, args.ProjectID, "progress"); err == nil && existing != nil {
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
		return map[string]any{
			"status": "error", "code": "read_only", "rule": "dirty_workspace",
			"message": "Authority worktree is dirty", "dirty_paths": dirty, "audit_state": "recorded",
		}
	}

	effectiveDate := s.todayLocal()
	reader, err := vaultread.Open(s.root)
	if err != nil {
		return mutationError("validation_failed", "vault", err.Error())
	}
	if !readerHasProject(reader, args.ProjectID) {
		_ = reader.Close()
		return mutationError("validation_failed", "project_id", "unknown project")
	}
	previousCommit := reader.SourceCommit()
	_ = reader.Close()
	rolePath, err := rolePathAtWorktree(s.root, args.ProjectID, "progress")
	if err != nil {
		return mutationError("validation_failed", "progress", err.Error())
	}

	_, statErr := os.Stat(rolePath)
	var raw []byte
	previousRevision := ""
	if os.IsNotExist(statErr) {
		raw = renderDefaultRoleFile("进度", args.ProjectID, effectiveDate)
	} else if statErr != nil {
		return mutationError("mutation_failed", "progress", fmt.Sprintf("inspect progress resource: %v", statErr))
	} else {
		rel, relErr := filepath.Rel(s.root, rolePath)
		if relErr != nil {
			return mutationError("mutation_failed", "progress", fmt.Sprintf("resolve progress resource: %v", relErr))
		}
		raw, err = readAuthorityAtCommit(s.root, previousCommit, rel)
		if err != nil {
			return mutationError("mutation_failed", "progress", fmt.Sprintf("read progress resource at Authority commit: %v", err))
		}
		if !utf8.Valid(raw) {
			return mutationError("validation_failed", "progress", "progress resource is not valid UTF-8")
		}
		previousRevision = revision(raw)
		if canonicalBodyPresent(string(raw), args.Content) {
			return mutationError("validation_failed", "duplicate_entry", "equivalent append already exists in the current resource")
		}
	}
	raw = canonicalManagedText(raw)
	operationID, err := randomOperationID()
	if err != nil {
		return mutationError("mutation_failed", "operation_id", err.Error())
	}
	marker := strings.TrimPrefix(payloadHash, "sha256:")
	candidateContent, err := renderProgress(raw, args.Summary, args.Content, marker, effectiveDate)
	if err != nil {
		return mutationError("validation_failed", "progress", err.Error())
	}
	candidateContent = canonicalManagedText(candidateContent)
	newRevision := revision(candidateContent)
	relativePath, err := filepath.Rel(s.root, rolePath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return mutationError("validation_failed", "path", "progress resource escapes Vault root")
	}
	relativePath = filepath.ToSlash(relativePath)

	candidateRoot := filepath.Join(s.workRoot, "candidate-"+operationID)
	defer os.RemoveAll(candidateRoot)
	if err := copyAuthorityTree(s.root, candidateRoot); err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
	}
	changes, err := s.normalRoleMutationChanges(args.ProjectID, relativePath, candidateContent, effectiveDate, false)
	if err != nil {
		return mutationError("validation_failed", "index", err.Error())
	}
	if err := applyCandidateChanges(candidateRoot, changes); err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
	}
	if err := vaultvalidate.ValidateAuthorityRoot(candidateRoot); err != nil {
		return mutationError("validation_failed", "vault", err.Error())
	}

	candidateCommit, err := s.buildCandidateCommitChanges(previousCommit, changes, operationID, "append_progress", args.ProjectID)
	if err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
	}
	plannedResult := committedResult(args, operationID, callerKeyHash, previousCommit, candidateCommit, previousRevision, newRevision, s.writeSource)
	journal := journalRecord{
		SchemaVersion: 2, OperationID: operationID, Operation: "append_progress", ProjectID: args.ProjectID,
		Role: "progress", State: "prepared", PreviousCommit: previousCommit, CandidateCommit: candidateCommit,
		RelativePath: relativePath, PayloadHash: payloadHash, CallerKeyHash: callerKeyHash,
		HasEntryMarker: true, Result: cloneMap(plannedResult),
	}
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
	record := operationRecord{
		SchemaVersion: 2, OperationID: operationID, Operation: "append_progress", ProjectID: args.ProjectID,
		Role: "progress", Status: "committed", ResultingCommit: candidateCommit, PayloadHash: payloadHash,
		CallerKeyHash: callerKeyHash, HasEntryMarker: true, Result: cloneMap(result),
	}
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

func (s *Service) Status(indexHead string) map[string]any {
	if s.indexHead != "" {
		indexHead = s.indexHead
	}
	head, _ := gitHead(s.root)
	clean, dirty, _ := gitWorktreeState(s.root)
	writeMode, readOnly, primaryActive, _, state, primary := s.writeModeState()
	pending, _ := s.pendingJournalCount()
	indexRevision, _ := s.currentIndexRevision()
	return map[string]any{
		"workspace_clean":     clean,
		"dirty_paths":         dirty,
		"read_only":           readOnly,
		"write_enabled":       s.writeEnabled,
		"write_mode":          writeMode,
		"git_head":            head,
		"index_head":          indexHead,
		"index_state":         state,
		"index_revision":      indexRevision,
		"write_journal_count": pending,
		"primary_active":      primaryActive,
		"primary":             primary,
		"startup_recovery":    s.RecoverySummary(),
	}
}

func (s *Service) Observe(indexHead, operationID string) map[string]any {
	if s.indexHead != "" {
		indexHead = s.indexHead
	}
	head, _ := gitHead(s.root)
	clean, dirty, _ := gitWorktreeState(s.root)
	writeMode, _, _, _, state, _ := s.writeModeState()
	pending, _ := s.pendingJournalCount()
	records, _ := s.operationRecords()
	var selected any
	if operationID != "" {
		for _, record := range records {
			if record.OperationID == operationID {
				selected = map[string]any{
					"operation_id":     record.OperationID,
					"operation":        record.Operation,
					"project_id":       record.ProjectID,
					"role":             record.Role,
					"status":           record.Status,
					"resulting_commit": record.ResultingCommit,
					"has_entry_marker": record.HasEntryMarker,
					"result":           record.Result,
				}
				break
			}
		}
	}
	return map[string]any{
		"git_head":          head,
		"worktree":          map[string]any{"clean": clean, "dirty_paths": dirty},
		"journal":           map[string]any{"pending_count": pending, "write_mode": writeMode},
		"index":             map[string]any{"state": state, "indexed_head": indexHead},
		"operation_records": map[string]any{"count": len(records), "selected": selected},
		"recovery":          s.RecoverySummary(),
	}
}

func (s *Service) nowUTC() time.Time  { return s.now().UTC() }
func (s *Service) todayLocal() string { return s.now().Format("2006-01-02") }

func (s *Service) currentIndexRevision() (string, error) {
	head, err := gitHead(s.root)
	if err != nil {
		return "", err
	}
	rel := "INDEX.yaml"
	if s.legacyIndex {
		rel = "INDEX.md"
	}
	raw, err := readAuthorityAtCommit(s.root, head, rel)
	if err != nil {
		return "", err
	}
	return revision(raw), nil
}

func validateAppendArgs(args AppendProgressArgs) error {
	if strings.TrimSpace(args.ProjectID) == "" {
		return fmt.Errorf("project_id is required")
	}
	if strings.ContainsAny(args.ProjectID, `/\\`) || args.ProjectID == "." || args.ProjectID == ".." {
		return fmt.Errorf("project_id is invalid")
	}
	if strings.TrimSpace(args.Summary) == "" || strings.ContainsAny(args.Summary, "\r\n") {
		return fmt.Errorf("summary must be a non-empty single line")
	}
	if strings.TrimSpace(args.Content) == "" || !utf8.ValidString(args.Content) {
		return fmt.Errorf("content must be non-empty UTF-8")
	}
	return nil
}

func normalizeProgressContent(content string) string {
	lines := strings.Split(textcanon.CanonicalText(content), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 {
		trimmed := strings.TrimSpace(lines[0])
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			lines = lines[1:]
			for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
				lines = lines[1:]
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func containsProgressStructuralHeading(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			return true
		}
	}
	return false
}

func committedResult(args AppendProgressArgs, operationID, callerKeyHash, previousCommit, newCommit, previousRevision, newRevision, source string) map[string]any {
	var previousRevisionValue any
	if previousRevision != "" {
		previousRevisionValue = previousRevision
	}
	return map[string]any{
		"status": "committed", "code": nil, "message": nil, "rule": nil,
		"project_id": args.ProjectID, "resource_uri": fmt.Sprintf("memory://projects/%s/progress", args.ProjectID),
		"previous_commit": previousCommit, "new_commit": newCommit,
		"previous_revision": previousRevisionValue, "new_revision": newRevision, "content_hash": newRevision,
		"operation_id": operationID, "caller_key_hash": callerKeyHash,
		"incremental_validation": "pass", "index_state": "current", "backup_state": "disabled",
		"idempotent_replay": false, "source": source, "evidence_count": 0, "audit_state": "recorded",
		"all_rolled_back": nil, "archive_reason": nil, "archived_at": nil, "available_sections": nil,
		"current_revision": nil, "failed_operation": nil, "failed_operation_index": nil, "runtime_status": nil,
	}
}

func mutationError(code, rule, message string) map[string]any {
	var ruleValue any
	if rule != "" {
		ruleValue = rule
	}
	return map[string]any{
		"status": "error", "code": code, "message": message, "rule": ruleValue, "audit_state": "recorded",
		"all_rolled_back": nil, "archive_reason": nil, "archived_at": nil, "available_sections": nil,
		"backup_state": nil, "caller_key_hash": nil, "content_hash": nil, "current_revision": nil,
		"evidence_count": nil, "failed_operation": nil, "failed_operation_index": nil,
		"idempotent_replay": nil, "incremental_validation": nil, "index_state": nil,
		"new_commit": nil, "new_revision": nil, "operation_id": nil, "previous_commit": nil,
		"previous_revision": nil, "project_id": nil, "resource_uri": nil, "runtime_status": nil, "source": nil,
	}
}

func replayResult(original map[string]any) map[string]any {
	out := cloneMap(original)
	out["idempotent_replay"] = true
	return out
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func renderProgress(raw []byte, summary, content, marker, date string) ([]byte, error) {
	text := string(raw)
	lineEnd := strings.IndexByte(text, '\n')
	if lineEnd < 0 {
		return nil, fmt.Errorf("progress resource has no Markdown body")
	}
	bodyStart := strings.Index(text[lineEnd+1:], "---\n")
	if strings.HasPrefix(text, "---\n") {
		if bodyStart < 0 {
			return nil, fmt.Errorf("progress frontmatter is not closed")
		}
		frontEnd := lineEnd + 1 + bodyStart + len("---\n")
		front := text[:frontEnd]
		front = replaceFrontmatterDate(front, "最后更新", date)
		text = front + text[frontEnd:]
	}
	insert, err := roleRootH1InsertOffset(text, "进度")
	if err != nil {
		return nil, err
	}
	entry := fmt.Sprintf("\n## %s\n\n**做了什么**：\n- %s\n\n%s\n\n<!-- v-memory-entry:%s -->\n", date, strings.TrimSpace(summary), strings.TrimSpace(content), marker)
	return []byte(text[:insert] + entry + text[insert:]), nil
}

func replaceFrontmatterDate(front, key, value string) string {
	prefix := key + ":"
	lines := strings.SplitAfter(front, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			newline := "\n"
			if !strings.HasSuffix(line, "\n") {
				newline = ""
			}
			lines[i] = key + ": " + value + newline
			break
		}
	}
	return strings.Join(lines, "")
}

func (s *Service) buildCandidateCommit(previousCommit, relativePath string, content []byte, operationID, projectID string) (string, error) {
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
	message := fmt.Sprintf("v-memory: append progress %s\n", projectID)
	commit, err := gitInput(s.root, serviceGitIdentityEnv(nil), []byte(message), "commit-tree", tree, "-p", previousCommit)
	if err != nil {
		return "", fmt.Errorf("create candidate commit: %w", err)
	}
	return commit, nil
}

const (
	serviceGitName  = "V-Memory MCP"
	serviceGitEmail = "v-memory@v-memory.invalid"
)

func serviceGitIdentityEnv(base []string) []string {
	env := append([]string(nil), base...)
	if env == nil {
		env = append([]string(nil), os.Environ()...)
	}
	return append(env,
		"GIT_AUTHOR_NAME="+serviceGitName,
		"GIT_AUTHOR_EMAIL="+serviceGitEmail,
		"GIT_COMMITTER_NAME="+serviceGitName,
		"GIT_COMMITTER_EMAIL="+serviceGitEmail,
	)
}

func canonicalManagedText(raw []byte) []byte {
	return []byte(textcanon.CanonicalText(string(raw)))
}

func canonicalBodyPresent(resource, body string) bool {
	needle := strings.TrimSpace(textcanon.CanonicalText(body))
	return needle != "" && strings.Contains(textcanon.CanonicalText(resource), needle)
}

func readerHasProject(reader *vaultread.Service, projectID string) bool {
	for _, project := range reader.Projects() {
		if project.ID == projectID {
			return true
		}
	}
	return false
}

func renderDefaultRoleFile(title, projectID, dateText string) []byte {
	descriptions := map[string]string{
		"进度": "时间倒序的阶段性进展。新内容追加在顶部。",
		"踩坑": "问题、症状、原因和解决记录。新内容追加在顶部。",
		"规范": "项目级长期规则。",
	}
	description, ok := descriptions[title]
	if !ok {
		return nil
	}
	text := fmt.Sprintf("---\n项目: %s\n创建: %s\n最后更新: %s\n最后核验: 未核验\n说明: %s\n---\n# %s - %s\n\n", projectID, dateText, dateText, description, title, projectID)
	return canonicalManagedText([]byte(text))
}

func roleRootH1InsertOffset(text, label string) (int, error) {
	start := 0
	if strings.HasPrefix(text, "---\n") {
		endRel := strings.Index(text[4:], "\n---\n")
		if endRel < 0 {
			return 0, fmt.Errorf("%s resource frontmatter is not closed", roleEnglishName(label))
		}
		start = 4 + endRel + len("\n---\n")
	}
	offset := start
	for _, line := range strings.SplitAfter(text[start:], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			valid := heading == label
			if strings.HasPrefix(heading, label+" - ") && strings.TrimSpace(strings.TrimPrefix(heading, label+" - ")) != "" {
				valid = true
			}
			if !valid {
				return 0, fmt.Errorf("%s resource has invalid root H1", roleEnglishName(label))
			}
			return offset + len(line), nil
		}
		offset += len(line)
	}
	return 0, fmt.Errorf("%s resource is missing # %s heading", roleEnglishName(label), label)
}

func roleEnglishName(label string) string {
	switch label {
	case "进度":
		return "progress"
	case "踩坑":
		return "pitfalls"
	case "交接":
		return "handoff"
	case "规范":
		return "rules"
	default:
		return "role"
	}
}

func gitInput(root string, env []string, input []byte, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...)
	if env != nil {
		cmd.Env = env
	}
	if input != nil {
		cmd.Stdin = strings.NewReader(string(input))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCASHead(root, oldHead, newHead string) error {
	_, err := gitInput(root, nil, nil, "update-ref", "HEAD", newHead, oldHead)
	return err
}

func gitResetHard(root, commit string) error {
	_, err := gitInput(root, nil, nil, "-c", "core.autocrlf=false", "-c", "core.eol=lf", "reset", "--hard", commit)
	return err
}

func readAuthorityAtCommit(root, commit, relativePath string) ([]byte, error) {
	return gitview.ReadFile(root, commit, filepath.ToSlash(relativePath))
}

func materializeCandidate(root, commit string, changes map[string]*[]byte) error {
	if err := gitResetHard(root, commit); err != nil {
		return err
	}
	if err := applyCandidateChanges(root, changes); err != nil {
		return fmt.Errorf("write canonical candidate bytes: %w", err)
	}
	clean, dirty, err := gitWorktreeState(root)
	if err != nil {
		return fmt.Errorf("verify materialized worktree: %w", err)
	}
	if !clean {
		return fmt.Errorf("materialized worktree differs from candidate commit: %s", strings.Join(dirty, ", "))
	}
	return nil
}

func gitHead(root string) (string, error) {
	return gitInput(root, nil, nil, "rev-parse", "HEAD")
}

func gitWorktreeState(root string) (bool, []string, error) {
	out, err := gitInput(root, nil, nil, "-c", "core.quotePath=false", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return false, nil, err
	}
	if strings.TrimSpace(out) == "" {
		return true, []string{}, nil
	}
	lines := strings.Split(out, "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return false, paths, nil
}

func (s *Service) rollbackAfterIndexFailure(previousCommit, candidateCommit, journalPath string, journal *journalRecord) error {
	if err := gitCASHead(s.root, candidateCommit, previousCommit); err != nil {
		return err
	}
	if err := gitResetHard(s.root, previousCommit); err != nil {
		return err
	}
	reader, err := vaultread.OpenWithOptions(s.root, vaultread.OpenOptions{ProjectionPath: s.projection, ForceRebuild: true})
	if err == nil {
		s.indexHead = reader.IndexHead()
		_ = reader.Close()
	}
	journal.State = "rolled_back"
	return writeJSONDurable(journalPath, *journal)
}

func copyAuthorityTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		mode := info.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("candidate Authority contains symlink: %s", rel)
		}
		if info.IsDir() {
			return os.MkdirAll(target, mode.Perm())
		}
		if !mode.IsRegular() {
			return fmt.Errorf("candidate Authority contains non-regular file: %s", rel)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func writeJSONDurable(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create durable state directory: %w", err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal durable state: %w", err)
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open durable temp file: %w", err)
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("write durable temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync durable temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close durable temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install durable state file: %w", err)
	}
	if err := syncDurableDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	ok = true
	return nil
}

func (s *Service) findByCallerKey(hash string) (*operationRecord, error) {
	records, err := s.operationRecords()
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].CallerKeyHash == hash {
			return &records[i], nil
		}
	}
	return nil, nil
}

func (s *Service) operationRecords() ([]operationRecord, error) {
	entries, err := os.ReadDir(s.operationsDir)
	if errors.Is(err, os.ErrNotExist) {
		return []operationRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]operationRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.operationsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record operationRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
		if record.Status == "committed" {
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	return out, nil
}

func (s *Service) pendingJournalCount() (int, error) {
	entries, err := os.ReadDir(s.journalDir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.journalDir, entry.Name()))
		if err != nil {
			return 0, err
		}
		var record journalRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return 0, err
		}
		switch record.State {
		case "finalized", "rolled_back", "aborted", "discarded", "recovered":
		default:
			count++
		}
	}
	return count, nil
}

func revision(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hashPayload(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return revision(raw), nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func randomOperationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
