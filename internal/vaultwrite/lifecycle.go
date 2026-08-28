// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iasi777/v-memory/internal/textcanon"
	"github.com/iasi777/v-memory/internal/vaultread"
	"github.com/iasi777/v-memory/internal/vaultvalidate"
	"gopkg.in/yaml.v3"
)

type CreateProjectArgs struct {
	ProjectID             string          `json:"project_id" jsonschema:"Portable registered project id."`
	Description           string          `json:"description" jsonschema:"Short project description stored in INDEX.yaml."`
	Lifecycle             string          `json:"lifecycle" jsonschema:"Must be active for a newly created project."`
	ExpectedIndexRevision string          `json:"expected_index_revision" jsonschema:"Current SHA-256 INDEX revision used for compare-and-swap."`
	Aliases               []string        `json:"aliases,omitempty" jsonschema:"Optional unique routing aliases for the project."`
	LocalPath             *string         `json:"local_path,omitempty" jsonschema:"Optional user-facing local path metadata; V-Memory does not read arbitrary host files from this value."`
	CreateRulesRole       bool            `json:"create_rules_role,omitempty" jsonschema:"If true, also create an empty canonical rules role; handoff is always created."`
	Evidence              []EvidenceInput `json:"evidence,omitempty" jsonschema:"Optional structured evidence metadata for the mutation; do not place secrets in durable content."`
	ClientIdempotencyKey  string          `json:"client_idempotency_key,omitempty" jsonschema:"Optional stable caller key for safe replay of the same mutation payload."`
}

type ArchiveProjectArgs struct {
	ProjectID             string `json:"project_id"`
	Reason                string `json:"reason"`
	ExpectedIndexRevision string `json:"expected_index_revision"`
	ClientIdempotencyKey  string `json:"client_idempotency_key,omitempty"`
}

type RestoreProjectArgs struct {
	ProjectID             string `json:"project_id"`
	ExpectedIndexRevision string `json:"expected_index_revision"`
	ClientIdempotencyKey  string `json:"client_idempotency_key,omitempty"`
}

type LifecycleMigrationProjectInput struct {
	ProjectID       string `json:"project_id" yaml:"project_id"`
	SourceLifecycle string `json:"source_lifecycle" yaml:"source_lifecycle"`
	TargetLifecycle string `json:"target_lifecycle" yaml:"target_lifecycle"`
}

type LifecycleMigrationManifestInput struct {
	SchemaVersion         int                              `json:"schema_version" yaml:"schema_version"`
	ExpectedIndexRevision string                           `json:"expected_index_revision" yaml:"expected_index_revision"`
	ArchiveReason         string                           `json:"archive_reason" yaml:"archive_reason"`
	Projects              []LifecycleMigrationProjectInput `json:"projects" yaml:"projects"`
}

type MigrateLifecycleArgs struct {
	Manifest             LifecycleMigrationManifestInput `json:"manifest"`
	ClientIdempotencyKey string                          `json:"client_idempotency_key,omitempty"`
}

type lifecycleIndex struct {
	SchemaVersion int                         `yaml:"schema_version"`
	Projects      map[string]lifecycleProject `yaml:"projects"`
}

type lifecycleProject struct {
	Description     string   `yaml:"description"`
	Lifecycle       string   `yaml:"lifecycle"`
	Aliases         []string `yaml:"aliases"`
	RuntimeResource *string  `yaml:"runtime_resource"`
	LocalPath       *string  `yaml:"local_path,omitempty"`
	LastUpdated     string   `yaml:"last_updated"`
	LastVerified    string   `yaml:"last_verified"`
	ArchivedAt      *string  `yaml:"archived_at,omitempty"`
	ArchiveReason   *string  `yaml:"archive_reason,omitempty"`
}

type legacyLifecycleProject struct {
	ProjectID    string
	Description  string
	Lifecycle    string
	Aliases      []string
	LocalPath    *string
	LastUpdated  string
	LastVerified string
}

var legacyIndexLineRE = regexp.MustCompile(`^- (🟢|🟡|🔵|🗄️) \*\*([^/]+)/\*\* → (.+)$`)

func (s *Service) LegacyLifecycleReady() bool { return s.legacyIndex }

func (s *Service) CreateProject(args CreateProjectArgs) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return preflight
	}
	if invalidLifecycleProjectID(args.ProjectID) {
		return mutationError("validation_failed", "project_id_path", "project_id is invalid")
	}
	if args.Lifecycle != "active" {
		return mutationError("validation_failed", "lifecycle", "new project lifecycle must be active")
	}
	if err := validateLifecycleText(args.Description, 500, "description", true); err != nil {
		return mutationError("validation_failed", "description", err.Error())
	}
	if !validRevision(args.ExpectedIndexRevision) {
		return mutationError("validation_failed", "expected_index_revision", "expected_index_revision must be a SHA-256 revision")
	}
	aliases, err := validateAliases(args.Aliases)
	if err != nil {
		return mutationError("validation_failed", "aliases", err.Error())
	}
	if args.LocalPath != nil {
		if err := validateLifecycleText(*args.LocalPath, 500, "local_path", true); err != nil {
			return mutationError("validation_failed", "local_path", err.Error())
		}
	}
	payload := map[string]any{
		"operation": "create_project", "project_id": args.ProjectID, "description": args.Description,
		"lifecycle": args.Lifecycle, "expected_index_revision": args.ExpectedIndexRevision,
		"aliases": aliases, "local_path": args.LocalPath, "create_rules_role": args.CreateRulesRole,
		"evidence": args.Evidence,
	}
	if payloadContainsHighConfidenceSecret(payload) {
		return mutationError("validation_failed", "content_secret", "durable mutation content contains high-confidence secret material")
	}
	payloadHash, _ := hashPayload(payload)
	callerKeyHash := hashOptionalKey(args.ClientIdempotencyKey)
	legacyPayload := map[string]any{"description": p5LegacyCleanText(args.Description), "lifecycle": args.Lifecycle, "local_path": p5LegacyOptionalString(args.LocalPath), "aliases": p5LegacyStrings(aliases), "expected_index_revision": args.ExpectedIndexRevision, "evidence": p5LegacyEvidence(args.Evidence)}
	if args.CreateRulesRole {
		legacyPayload["create_rules_role"] = true
	}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, args.ClientIdempotencyKey, payloadHash, args.ProjectID, "handoff", "create_project", legacyPayload); handled {
		return result
	}

	indexRaw, indexDoc, indexRevision, err := s.readSchema4Index()
	if err != nil {
		return mutationError("validation_failed", "index", err.Error())
	}
	if args.ExpectedIndexRevision != indexRevision {
		return conflictWithCurrent("INDEX revision does not match current content", indexRevision)
	}
	if _, exists := indexDoc.Projects[args.ProjectID]; exists {
		return mutationError("validation_failed", "index_target", "project already exists")
	}
	date := s.todayLocal()
	handoff := renderProjectHandoff(args.ProjectID, date)
	rules := renderEmptyRules(args.ProjectID, date)
	project := lifecycleProject{
		Description: args.Description, Lifecycle: "active", Aliases: aliases, RuntimeResource: nil,
		LocalPath: args.LocalPath, LastUpdated: date, LastVerified: "未核验",
	}
	indexDoc.Projects[args.ProjectID] = project
	newIndex, err := yaml.Marshal(indexDoc)
	if err != nil {
		return mutationError("mutation_failed", "index", err.Error())
	}
	changes := map[string]*[]byte{
		"INDEX.yaml": bytesPtr(newIndex), filepath.ToSlash(filepath.Join(args.ProjectID, "交接.md")): bytesPtr(handoff),
	}
	if args.CreateRulesRole {
		changes[filepath.ToSlash(filepath.Join(args.ProjectID, "规范.md"))] = bytesPtr(rules)
	}
	_ = indexRaw
	previousCommit, _ := gitHead(s.root)
	resultSpec := lifecycleResultSpec{
		Operation: "create_project", ProjectID: args.ProjectID, Role: "handoff", ResourceURI: fmt.Sprintf("memory://projects/%s/handoff", args.ProjectID),
		PreviousRevision: "", NewRevision: revision(handoff), EvidenceCount: len(args.Evidence),
	}
	return s.commitLifecycleChanges(resultSpec, payloadHash, callerKeyHash, previousCommit, changes)
}

func (s *Service) ArchiveProject(args ArchiveProjectArgs) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return preflight
	}
	if invalidLifecycleProjectID(args.ProjectID) {
		return mutationError("validation_failed", "project_id_path", "project_id is invalid")
	}
	if err := validateLifecycleText(args.Reason, 500, "archive_reason", true); err != nil || strings.Contains(args.Reason, "｜") {
		return mutationError("validation_failed", "archive_reason", "archive reason is invalid")
	}
	if !validRevision(args.ExpectedIndexRevision) {
		return mutationError("validation_failed", "expected_index_revision", "expected_index_revision must be a SHA-256 revision")
	}
	_, doc, currentRevision, err := s.readSchema4Index()
	if err != nil {
		return mutationError("validation_failed", "index", err.Error())
	}
	project, ok := doc.Projects[args.ProjectID]
	if !ok {
		return mutationError("validation_failed", "index_target", "project is absent from INDEX")
	}
	callerKeyHash := hashOptionalKey(args.ClientIdempotencyKey)
	if project.Lifecycle == "archived" {
		if project.ArchiveReason != nil && *project.ArchiveReason == strings.TrimSpace(args.Reason) {
			return alreadyArchivedResult(args.ProjectID, currentRevision, project, callerKeyHash, s.writeSource)
		}
		return mutationError("validation_failed", "archive_reason", "project is already archived with a different reason")
	}
	if args.ExpectedIndexRevision != currentRevision {
		return conflictWithCurrent("INDEX revision does not match current content", currentRevision)
	}
	payload := map[string]any{"operation": "archive_project", "project_id": args.ProjectID, "reason": strings.TrimSpace(args.Reason), "expected_index_revision": args.ExpectedIndexRevision}
	payloadHash, _ := hashPayload(payload)
	legacyPayload := map[string]any{"reason": p5LegacyCleanText(args.Reason), "expected_index_revision": args.ExpectedIndexRevision, "evidence": []EvidenceInput{}}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, args.ClientIdempotencyKey, payloadHash, args.ProjectID, "index", "archive_project", legacyPayload); handled {
		return result
	}
	archivedAt := s.nowUTC().Format(time.RFC3339)
	reason := strings.TrimSpace(args.Reason)
	project.Lifecycle = "archived"
	project.RuntimeResource = nil
	project.ArchivedAt = &archivedAt
	project.ArchiveReason = &reason
	doc.Projects[args.ProjectID] = project
	newIndex, err := yaml.Marshal(doc)
	if err != nil {
		return mutationError("mutation_failed", "index", err.Error())
	}
	previousCommit, _ := gitHead(s.root)
	spec := lifecycleResultSpec{Operation: "archive_project", ProjectID: args.ProjectID, Role: "index", ResourceURI: "memory://index", PreviousRevision: currentRevision, NewRevision: revision(newIndex)}
	return s.commitLifecycleChanges(spec, payloadHash, callerKeyHash, previousCommit, map[string]*[]byte{"INDEX.yaml": bytesPtr(newIndex)})
}

func (s *Service) RestoreProject(args RestoreProjectArgs) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return preflight
	}
	if invalidLifecycleProjectID(args.ProjectID) || !validRevision(args.ExpectedIndexRevision) {
		return mutationError("validation_failed", "arguments", "invalid restore arguments")
	}
	_, doc, currentRevision, err := s.readSchema4Index()
	if err != nil {
		return mutationError("validation_failed", "index", err.Error())
	}
	project, ok := doc.Projects[args.ProjectID]
	if !ok {
		return mutationError("validation_failed", "index_target", "project is absent from INDEX")
	}
	callerKeyHash := hashOptionalKey(args.ClientIdempotencyKey)
	if project.Lifecycle == "active" {
		return alreadyActiveResult(args.ProjectID, currentRevision, callerKeyHash, s.writeSource)
	}
	if args.ExpectedIndexRevision != currentRevision {
		return conflictWithCurrent("INDEX revision does not match current content", currentRevision)
	}
	payload := map[string]any{"operation": "restore_project", "project_id": args.ProjectID, "expected_index_revision": args.ExpectedIndexRevision}
	payloadHash, _ := hashPayload(payload)
	legacyPayload := map[string]any{"expected_index_revision": args.ExpectedIndexRevision, "evidence": []EvidenceInput{}}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, args.ClientIdempotencyKey, payloadHash, args.ProjectID, "index", "restore_project", legacyPayload); handled {
		return result
	}
	project.Lifecycle = "active"
	project.ArchivedAt = nil
	project.ArchiveReason = nil
	runtimeStatus := "runtime_missing"
	runtimePath := filepath.Join(s.root, "projects", args.ProjectID, "runtime.yaml")
	if _, err := os.Stat(runtimePath); err == nil {
		value := filepath.ToSlash(filepath.Join("projects", args.ProjectID, "runtime.yaml"))
		project.RuntimeResource = &value
		runtimeStatus = "ok"
	} else {
		project.RuntimeResource = nil
	}
	doc.Projects[args.ProjectID] = project
	newIndex, err := yaml.Marshal(doc)
	if err != nil {
		return mutationError("mutation_failed", "index", err.Error())
	}
	previousCommit, _ := gitHead(s.root)
	spec := lifecycleResultSpec{Operation: "restore_project", ProjectID: args.ProjectID, Role: "index", ResourceURI: "memory://index", PreviousRevision: currentRevision, NewRevision: revision(newIndex), Extra: map[string]any{"runtime_status": runtimeStatus}}
	return s.commitLifecycleChanges(spec, payloadHash, callerKeyHash, previousCommit, map[string]*[]byte{"INDEX.yaml": bytesPtr(newIndex)})
}

func (s *Service) MigrateLifecycle(args MigrateLifecycleArgs) map[string]any {
	if err := validateMigrationManifest(args.Manifest); err != nil {
		return mutationError("validation_failed", err.rule, err.message)
	}
	callerKeyHash := hashOptionalKey(args.ClientIdempotencyKey)
	if raw, _, currentRevision, err := s.readSchema4Index(); err == nil && len(raw) > 0 {
		head, _ := gitHead(s.root)
		return alreadyMigratedResult(currentRevision, head, callerKeyHash, s.writeSource)
	}
	if preflight := s.writePreflightLegacyMigration(); preflight != nil {
		return preflight
	}
	legacyRaw, err := os.ReadFile(filepath.Join(s.root, "INDEX.md"))
	if err != nil {
		return mutationError("validation_failed", "migration_state", "legacy lifecycle INDEX is required")
	}
	legacyRevision := revision(legacyRaw)
	if args.Manifest.ExpectedIndexRevision != legacyRevision {
		return conflictWithCurrent("INDEX revision does not match lifecycle manifest", legacyRevision)
	}
	records, err := parseLegacyLifecycleIndex(legacyRaw)
	if err != nil {
		return mutationError("validation_failed", "migration_state", err.Error())
	}
	decisions := make(map[string]LifecycleMigrationProjectInput, len(args.Manifest.Projects))
	for _, decision := range args.Manifest.Projects {
		decisions[decision.ProjectID] = decision
	}
	if len(decisions) != len(records) {
		return mutationError("validation_failed", "migration_manifest_complete", "manifest must cover every project")
	}
	for _, record := range records {
		decision, ok := decisions[record.ProjectID]
		if !ok {
			return mutationError("validation_failed", "migration_manifest_complete", "manifest must cover every project")
		}
		if decision.SourceLifecycle != record.Lifecycle {
			return mutationError("validation_failed", "migration_source_lifecycle", "source lifecycle drift")
		}
	}
	appliedAt := s.nowUTC().Format(time.RFC3339)
	indexDoc := lifecycleIndex{SchemaVersion: 4, Projects: make(map[string]lifecycleProject, len(records))}
	for _, record := range records {
		decision := decisions[record.ProjectID]
		lastUpdated, lastVerified := legacyRoleSummaryDates(s.root, record.ProjectID, record.LastUpdated, record.LastVerified)
		project := lifecycleProject{
			Description: record.Description, Aliases: record.Aliases, RuntimeResource: nil, LocalPath: record.LocalPath,
			LastUpdated: lastUpdated, LastVerified: lastVerified,
		}
		if decision.TargetLifecycle == "archived" {
			reason := args.Manifest.ArchiveReason
			project.Lifecycle = "archived"
			project.ArchivedAt = &appliedAt
			project.ArchiveReason = &reason
		} else {
			project.Lifecycle = "active"
		}
		indexDoc.Projects[record.ProjectID] = project
	}
	newIndex, err := yaml.Marshal(indexDoc)
	if err != nil {
		return mutationError("mutation_failed", "index", err.Error())
	}
	previousCommit, _ := gitHead(s.root)
	manifestDoc := struct {
		SchemaVersion         int                              `yaml:"schema_version"`
		ExpectedIndexRevision string                           `yaml:"expected_index_revision"`
		ArchiveReason         string                           `yaml:"archive_reason"`
		Projects              []LifecycleMigrationProjectInput `yaml:"projects"`
		SourceCommit          string                           `yaml:"source_commit"`
		AppliedAt             string                           `yaml:"applied_at"`
	}{args.Manifest.SchemaVersion, args.Manifest.ExpectedIndexRevision, args.Manifest.ArchiveReason, args.Manifest.Projects, previousCommit, appliedAt}
	manifestRaw, err := yaml.Marshal(manifestDoc)
	if err != nil {
		return mutationError("mutation_failed", "migration_manifest", err.Error())
	}
	payload := map[string]any{"operation": "migrate_lifecycle", "manifest": args.Manifest}
	payloadHash, _ := hashPayload(payload)
	legacyPayload := map[string]any{"manifest": p5LegacyLifecycleManifest(args.Manifest), "evidence": []EvidenceInput{}}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, args.ClientIdempotencyKey, payloadHash, "INDEX", "index", "migrate_lifecycle", legacyPayload); handled {
		return result
	}
	changes := map[string]*[]byte{
		"INDEX.md":                     nil,
		"INDEX.yaml":                   bytesPtr(newIndex),
		"migrations/lifecycle-v1.yaml": bytesPtr(manifestRaw),
		"migrations/lifecycle-v1-source-INDEX.md": bytesPtr(legacyRaw),
	}
	spec := lifecycleResultSpec{Operation: "migrate_lifecycle", ProjectID: "INDEX", Role: "index", ResourceURI: "memory://index", PreviousRevision: legacyRevision, NewRevision: revision(newIndex)}
	result := s.commitLifecycleChanges(spec, payloadHash, callerKeyHash, previousCommit, changes)
	if result["status"] == "committed" {
		s.legacyIndex = false
	}
	return result
}

func legacyRoleSummaryDates(root, projectID, fallbackUpdated, fallbackVerified string) (string, string) {
	maxUpdated := fallbackUpdated
	lastVerified := fallbackVerified
	for _, item := range []struct {
		filename string
		handoff  bool
	}{
		{"交接.md", true}, {"规范.md", false}, {"进度.md", false}, {"踩坑.md", false},
	} {
		raw, err := os.ReadFile(filepath.Join(root, projectID, item.filename))
		if err != nil || !utf8.Valid(raw) {
			continue
		}
		values := map[string]string{}
		for _, line := range strings.Split(textcanon.CanonicalText(string(raw)), "\n") {
			if line == "---" {
				continue
			}
			if strings.HasPrefix(line, "# ") {
				break
			}
			if at := strings.Index(line, ":"); at > 0 {
				values[strings.TrimSpace(line[:at])] = strings.TrimSpace(line[at+1:])
			}
		}
		if updated := values["最后更新"]; updated > maxUpdated {
			maxUpdated = updated
		}
		if item.handoff && values["最后核验"] != "" {
			lastVerified = values["最后核验"]
		}
	}
	return maxUpdated, lastVerified
}

type lifecycleResultSpec struct {
	Operation        string
	ProjectID        string
	Role             string
	ResourceURI      string
	PreviousRevision string
	NewRevision      string
	EvidenceCount    int
	Extra            map[string]any
	RelativePath     string
}

func (s *Service) commitLifecycleChanges(spec lifecycleResultSpec, payloadHash, callerKeyHash, previousCommit string, changes map[string]*[]byte) map[string]any {
	changes = canonicalizeLifecycleChanges(changes)
	if spec.ResourceURI == "memory://index" {
		if indexContent, ok := changes["INDEX.yaml"]; ok && indexContent != nil {
			spec.NewRevision = revision(*indexContent)
		}
	}
	operationID, err := randomOperationID()
	if err != nil {
		return mutationError("mutation_failed", "operation_id", err.Error())
	}
	candidateRoot := filepath.Join(s.workRoot, "candidate-"+operationID)
	defer os.RemoveAll(candidateRoot)
	if err := copyAuthorityTree(s.root, candidateRoot); err != nil {
		return mutationError("mutation_failed", "candidate", err.Error())
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
	result := committedLifecycleResult(spec, operationID, callerKeyHash, previousCommit, candidateCommit, s.writeSource)
	relativePath := spec.RelativePath
	if relativePath == "" {
		relativePath = "INDEX.yaml"
	}
	journal := journalRecord{
		SchemaVersion: 2, OperationID: operationID, Operation: spec.Operation, ProjectID: spec.ProjectID, Role: spec.Role,
		State: "prepared", PreviousCommit: previousCommit, CandidateCommit: candidateCommit, RelativePath: relativePath,
		PayloadHash: payloadHash, CallerKeyHash: callerKeyHash, HasEntryMarker: false, Result: cloneMap(result),
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
	s.maybeCrash("ref_updated_before_materialize")
	journal.State = "ref_updated"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	if err := materializeCandidate(s.root, candidateCommit, changes); err != nil {
		return mutationError("mutation_failed", "materialize", err.Error())
	}
	s.maybeCrash("materialized_before_index")
	journal.State = "materialized"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	reader, err := vaultread.OpenWithOptions(s.root, vaultread.OpenOptions{ProjectionPath: s.projection, ForceRebuild: true})
	if err != nil {
		journal.State = "index_failed"
		_ = writeJSONDurable(journalPath, journal)
		s.maybeCrash("index_failure_before_rollback")
		_ = s.rollbackAfterIndexFailure(previousCommit, candidateCommit, journalPath, &journal)
		return mutationError("mutation_failed", "index", err.Error())
	}
	indexedHead := reader.IndexHead()
	_ = reader.Close()
	if indexedHead != candidateCommit {
		journal.State = "index_failed"
		_ = writeJSONDurable(journalPath, journal)
		_ = s.rollbackAfterIndexFailure(previousCommit, candidateCommit, journalPath, &journal)
		return mutationError("mutation_failed", "index", "projection did not align with candidate commit")
	}
	s.indexHead = indexedHead
	journal.State = "indexed"
	if err := writeJSONDurable(journalPath, journal); err != nil {
		return mutationError("mutation_failed", "journal", err.Error())
	}
	record := operationRecord{SchemaVersion: 2, OperationID: operationID, Operation: spec.Operation, ProjectID: spec.ProjectID, Role: spec.Role, Status: "committed", ResultingCommit: candidateCommit, PayloadHash: payloadHash, CallerKeyHash: callerKeyHash, HasEntryMarker: false, Result: cloneMap(result)}
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

func committedLifecycleResult(spec lifecycleResultSpec, operationID, callerKeyHash, previousCommit, newCommit, source string) map[string]any {
	var previousRevision any
	if spec.PreviousRevision != "" {
		previousRevision = spec.PreviousRevision
	}
	out := map[string]any{
		"status": "committed", "code": nil, "message": nil, "rule": nil,
		"project_id": spec.ProjectID, "resource_uri": spec.ResourceURI,
		"previous_commit": previousCommit, "new_commit": newCommit,
		"previous_revision": previousRevision, "new_revision": spec.NewRevision, "content_hash": spec.NewRevision,
		"operation_id": operationID, "caller_key_hash": callerKeyHash,
		"incremental_validation": "pass", "index_state": "current", "backup_state": "disabled",
		"idempotent_replay": false, "source": source, "evidence_count": spec.EvidenceCount, "audit_state": "recorded",
		"all_rolled_back": nil, "archive_reason": nil, "archived_at": nil, "available_sections": nil,
		"current_revision": nil, "failed_operation": nil, "failed_operation_index": nil, "runtime_status": nil,
	}
	for k, v := range spec.Extra {
		out[k] = v
	}
	return out
}

func (s *Service) buildCandidateCommitChanges(previousCommit string, changes map[string]*[]byte, operationID, operation, projectID string) (string, error) {
	indexPath := filepath.Join(s.workRoot, "git-index-"+operationID)
	defer os.Remove(indexPath)
	env := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if _, err := gitInput(s.root, env, nil, "read-tree", previousCommit); err != nil {
		return "", fmt.Errorf("read candidate tree: %w", err)
	}
	paths := make([]string, 0, len(changes))
	for path := range changes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !safeAuthorityRelativePath(path) {
			return "", fmt.Errorf("unsafe Authority change path")
		}
		content := changes[path]
		if content == nil {
			if _, err := gitInput(s.root, env, nil, "update-index", "--force-remove", "--", path); err != nil {
				return "", fmt.Errorf("remove candidate path: %w", err)
			}
			continue
		}
		blob, err := gitInput(s.root, nil, *content, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", fmt.Errorf("write candidate blob: %w", err)
		}
		if _, err := gitInput(s.root, env, nil, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+path); err != nil {
			return "", fmt.Errorf("update candidate index: %w", err)
		}
	}
	tree, err := gitInput(s.root, env, nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write candidate tree: %w", err)
	}
	previousTree, err := gitInput(s.root, nil, nil, "rev-parse", previousCommit+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("read previous tree: %w", err)
	}
	if tree == previousTree {
		return previousCommit, nil
	}
	message := fmt.Sprintf("v-memory: %s %s\n", strings.ReplaceAll(operation, "_", " "), projectID)
	commit, err := gitInput(s.root, serviceGitIdentityEnv(nil), []byte(message), "commit-tree", tree, "-p", previousCommit)
	if err != nil {
		return "", fmt.Errorf("create candidate commit: %w", err)
	}
	return commit, nil
}

func canonicalizeLifecycleChanges(changes map[string]*[]byte) map[string]*[]byte {
	out := make(map[string]*[]byte, len(changes))
	for path, content := range changes {
		if content == nil {
			out[path] = nil
			continue
		}
		// The frozen migration source copy is audit evidence and must preserve
		// its historical bytes. Every newly managed text resource is canonical.
		if path == "migrations/lifecycle-v1-source-INDEX.md" {
			out[path] = bytesPtr(*content)
			continue
		}
		out[path] = bytesPtr(canonicalManagedText(*content))
	}
	return out
}

func applyCandidateChanges(root string, changes map[string]*[]byte) error {
	for path, content := range changes {
		if !safeAuthorityRelativePath(path) {
			return fmt.Errorf("unsafe Authority change path")
		}
		full := filepath.Join(root, filepath.FromSlash(path))
		if content == nil {
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, *content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func safeAuthorityRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == path && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "\\")
}

func (s *Service) readSchema4Index() ([]byte, lifecycleIndex, string, error) {
	var doc lifecycleIndex
	head, err := gitHead(s.root)
	if err != nil {
		return nil, doc, "", err
	}
	raw, err := readAuthorityAtCommit(s.root, head, "INDEX.yaml")
	if err != nil {
		return nil, doc, "", err
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, doc, "", err
	}
	if doc.SchemaVersion != 4 || doc.Projects == nil {
		return nil, doc, "", fmt.Errorf("current schema-4 INDEX.yaml is required")
	}
	return raw, doc, revision(raw), nil
}

// normalRoleMutationChanges builds the complete candidate tree for one of the
// four normal role resources. The role write and the project-level
// last_updated projection are one Git tree when the date changes. When the
// INDEX already carries the effective date, it is intentionally omitted from
// the change set so a same-day role mutation does not rewrite INDEX.yaml.
func (s *Service) normalRoleMutationChanges(projectID, relativePath string, content []byte, effectiveDate string, syncLastVerified bool) (map[string]*[]byte, error) {
	if !safeAuthorityRelativePath(relativePath) {
		return nil, fmt.Errorf("unsafe Authority role path")
	}
	if strings.TrimSpace(effectiveDate) == "" {
		return nil, fmt.Errorf("effective role mutation date is required")
	}
	_, indexDoc, _, err := s.readSchema4Index()
	if err != nil {
		return nil, err
	}
	project, ok := indexDoc.Projects[projectID]
	if !ok {
		return nil, fmt.Errorf("unknown project")
	}
	content = canonicalManagedText(content)
	changes := map[string]*[]byte{relativePath: bytesPtr(content)}
	targetLastUpdated := project.LastUpdated
	if targetLastUpdated == "" || effectiveDate > targetLastUpdated {
		targetLastUpdated = effectiveDate
	}
	indexChanged := project.LastUpdated != targetLastUpdated
	if syncLastVerified && project.LastVerified != effectiveDate {
		indexChanged = true
	}
	if !indexChanged {
		return changes, nil
	}
	project.LastUpdated = targetLastUpdated
	if syncLastVerified {
		project.LastVerified = effectiveDate
	}
	indexDoc.Projects[projectID] = project
	updatedIndex, err := yaml.Marshal(indexDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal synchronized INDEX.yaml: %w", err)
	}
	changes["INDEX.yaml"] = bytesPtr(canonicalManagedText(updatedIndex))
	return changes, nil
}

func parseLegacyLifecycleIndex(raw []byte) ([]legacyLifecycleProject, error) {
	out := make([]legacyLifecycleProject, 0)
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		m := legacyIndexLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := m[2]
		if invalidLifecycleProjectID(id) || seen[id] {
			return nil, fmt.Errorf("invalid or duplicate project in legacy INDEX")
		}
		seen[id] = true
		parts := strings.Split(m[3], "｜")
		description := strings.TrimSpace(parts[0])
		fields := map[string]string{}
		for _, part := range parts[1:] {
			sep := "："
			if !strings.Contains(part, sep) {
				sep = ":"
			}
			if i := strings.Index(part, sep); i >= 0 {
				fields[strings.TrimSpace(part[:i])] = strings.TrimSpace(part[i+len(sep):])
			}
		}
		aliases := []string{}
		for _, alias := range strings.Split(fields["别名"], ",") {
			if strings.TrimSpace(alias) != "" {
				aliases = append(aliases, strings.TrimSpace(alias))
			}
		}
		life := map[string]string{"🟢": "active", "🟡": "stable", "🔵": "dormant", "🗄️": "archived"}[m[1]]
		var localPath *string
		if value := fields["本地路径"]; value != "" {
			v := value
			localPath = &v
		}
		out = append(out, legacyLifecycleProject{ProjectID: id, Description: description, Lifecycle: life, Aliases: aliases, LocalPath: localPath, LastUpdated: fields["最后更新"], LastVerified: fields["最后核验"]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("legacy lifecycle INDEX is required")
	}
	return out, nil
}

func isLegacyLifecycleIndex(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "INDEX.yaml")); err == nil {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(root, "INDEX.md"))
	if err != nil {
		return false
	}
	_, err = parseLegacyLifecycleIndex(raw)
	return err == nil
}

func renderProjectHandoff(projectID, date string) []byte {
	text := fmt.Sprintf("---\n项目: %s\n创建: %s\n最后更新: %s\n最后核验: 未核验\n说明: 项目交接与当前接手上下文。\n---\n# 交接 - %s\n\n## 核验记录\n\n- 核验于: 未核验\n- 核验级别: 未核验\n- 核验范围: 未核验\n- 核验依据: 未核验\n- 核验基线: 未核验\n- 结果: 尚未核验\n- 未核验: 全部\n", projectID, date, date, projectID)
	return canonicalManagedText([]byte(text))
}

func renderEmptyRules(projectID, date string) []byte {
	return canonicalManagedText([]byte(fmt.Sprintf("---\n项目: %s\n创建: %s\n最后更新: %s\n最后核验: 未核验\n说明: 项目级长期规则。\n---\n# 规范 - %s\n\n", projectID, date, date, projectID)))
}

type migrationValidationError struct{ rule, message string }

func validateMigrationManifest(m LifecycleMigrationManifestInput) *migrationValidationError {
	if m.SchemaVersion != 1 {
		return &migrationValidationError{"migration_manifest_schema", "schema_version must be 1"}
	}
	if !validRevision(m.ExpectedIndexRevision) {
		return &migrationValidationError{"migration_manifest_revision", "expected_index_revision must be a SHA-256 revision"}
	}
	if err := validateLifecycleText(m.ArchiveReason, 500, "archive_reason", true); err != nil || strings.Contains(m.ArchiveReason, "｜") {
		return &migrationValidationError{"archive_reason", "archive_reason is invalid"}
	}
	if len(m.Projects) < 1 || len(m.Projects) > 512 {
		return &migrationValidationError{"migration_manifest_projects", "projects must contain 1 to 512 decisions"}
	}
	seen := map[string]bool{}
	for _, item := range m.Projects {
		if invalidLifecycleProjectID(item.ProjectID) || seen[item.ProjectID] {
			return &migrationValidationError{"migration_manifest_projects", "invalid or duplicate project decision"}
		}
		seen[item.ProjectID] = true
		if item.SourceLifecycle != "active" && item.SourceLifecycle != "stable" && item.SourceLifecycle != "dormant" {
			return &migrationValidationError{"migration_source_lifecycle", "invalid source lifecycle"}
		}
		if item.TargetLifecycle != "active" && item.TargetLifecycle != "archived" {
			return &migrationValidationError{"migration_target_lifecycle", "invalid target lifecycle"}
		}
	}
	return nil
}

func validateAliases(values []string) ([]string, error) {
	if len(values) > 12 {
		return nil, fmt.Errorf("too many aliases")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if err := validateLifecycleText(v, 100, "alias", true); err != nil {
			return nil, err
		}
		if seen[v] {
			return nil, fmt.Errorf("duplicate alias")
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}

func validateLifecycleText(value string, max int, field string, singleLine bool) error {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) {
		return fmt.Errorf("%s must be non-empty UTF-8", field)
	}
	if len([]rune(value)) > max {
		return fmt.Errorf("%s is too long", field)
	}
	if singleLine && strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must be a single line", field)
	}
	return nil
}

func invalidLifecycleProjectID(id string) bool {
	return invalidProjectIDPath(id) || strings.HasPrefix(id, ".") || strings.ContainsAny(id, "\n\r｜|")
}
func validRevision(value string) bool {
	return regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(value)
}
func hashOptionalKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return hashText(value)
}
func bytesPtr(value []byte) *[]byte { copyValue := append([]byte(nil), value...); return &copyValue }

func conflictWithCurrent(message, current string) map[string]any {
	out := mutationError("conflict", "", message)
	out["current_revision"] = current
	return out
}

func alreadyArchivedResult(projectID, revisionValue string, project lifecycleProject, callerKeyHash, source string) map[string]any {
	var archivedAt, reason any
	if project.ArchivedAt != nil {
		archivedAt = *project.ArchivedAt
	}
	if project.ArchiveReason != nil {
		reason = *project.ArchiveReason
	}
	return map[string]any{
		"status": "already_archived", "code": nil, "message": "project is already archived with the same reason", "rule": nil,
		"project_id": projectID, "resource_uri": "memory://index", "previous_revision": revisionValue, "new_revision": revisionValue, "content_hash": revisionValue,
		"previous_commit": nil, "new_commit": nil, "operation_id": nil, "caller_key_hash": callerKeyHash, "idempotent_replay": true,
		"incremental_validation": "not_run", "index_state": "current", "backup_state": "disabled", "source": source, "evidence_count": 0, "audit_state": "recorded",
		"archive_reason": reason, "archived_at": archivedAt, "all_rolled_back": nil, "available_sections": nil, "current_revision": nil, "failed_operation": nil, "failed_operation_index": nil, "runtime_status": nil,
	}
}

func alreadyActiveResult(projectID, revisionValue, callerKeyHash, source string) map[string]any {
	return map[string]any{
		"status": "already_active", "code": nil, "message": "project is already active", "rule": nil, "project_id": projectID, "resource_uri": "memory://index",
		"previous_revision": revisionValue, "new_revision": revisionValue, "content_hash": revisionValue, "previous_commit": nil, "new_commit": nil, "operation_id": nil,
		"caller_key_hash": callerKeyHash, "idempotent_replay": true, "incremental_validation": "not_run", "index_state": "current", "backup_state": "disabled", "source": source,
		"evidence_count": 0, "audit_state": "recorded", "archive_reason": nil, "archived_at": nil, "all_rolled_back": nil, "available_sections": nil, "current_revision": nil, "failed_operation": nil, "failed_operation_index": nil, "runtime_status": nil,
	}
}

func alreadyMigratedResult(revisionValue, head, callerKeyHash, source string) map[string]any {
	return map[string]any{
		"status": "already_migrated", "code": nil, "message": nil, "rule": nil, "project_id": "INDEX", "resource_uri": "memory://index",
		"previous_revision": revisionValue, "new_revision": revisionValue, "content_hash": revisionValue, "previous_commit": head, "new_commit": head, "operation_id": nil,
		"caller_key_hash": callerKeyHash, "idempotent_replay": true, "incremental_validation": "pass", "index_state": "current", "backup_state": "disabled", "source": source,
		"evidence_count": 0, "audit_state": "recorded", "archive_reason": nil, "archived_at": nil, "all_rolled_back": nil, "available_sections": nil, "current_revision": nil, "failed_operation": nil, "failed_operation_index": nil, "runtime_status": nil,
	}
}
