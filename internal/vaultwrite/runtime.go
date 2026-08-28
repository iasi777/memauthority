// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultwrite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/iasi777/v-memory/internal/runtimeview"
	"gopkg.in/yaml.v3"
)

type RuntimeRepositoryInput struct {
	Path   string  `json:"path" yaml:"path"`
	Branch string  `json:"branch" yaml:"branch"`
	Remote *string `json:"remote,omitempty" yaml:"remote,omitempty"`
}

type RuntimeSupervisorInput struct {
	Kind        string   `json:"kind" yaml:"kind"`
	Units       []string `json:"units,omitempty" yaml:"units,omitempty"`
	Services    []string `json:"services,omitempty" yaml:"services,omitempty"`
	ComposeFile *string  `json:"compose_file,omitempty" yaml:"compose_file,omitempty"`
	Containers  []string `json:"containers,omitempty" yaml:"containers,omitempty"`
	ProcessName *string  `json:"process_name,omitempty" yaml:"process_name,omitempty"`
	PIDFile     *string  `json:"pid_file,omitempty" yaml:"pid_file,omitempty"`
}

type RuntimeCheckInput struct {
	Kind        string   `json:"kind" yaml:"kind"`
	Target      *string  `json:"target,omitempty" yaml:"target,omitempty" jsonschema:"Optional for git_status and git_head; omitted values default to repository. If provided for those checks, the only supported value is repository."`
	Scope       *string  `json:"scope,omitempty" yaml:"scope,omitempty"`
	Units       []string `json:"units,omitempty" yaml:"units,omitempty"`
	Services    []string `json:"services,omitempty" yaml:"services,omitempty"`
	ComposeFile *string  `json:"compose_file,omitempty" yaml:"compose_file,omitempty"`
	Path        *string  `json:"path,omitempty" yaml:"path,omitempty"`
}

type RuntimeBoundariesInput struct {
	ProductionWriter                      bool  `json:"production_writer" yaml:"production_writer"`
	SingleWriter                          *bool `json:"single_writer,omitempty" yaml:"single_writer,omitempty"`
	MutationRequiresExplicitAuthorization bool  `json:"mutation_requires_explicit_authorization" yaml:"mutation_requires_explicit_authorization"`
}

type RuntimeEnvironmentInput struct {
	EnvironmentID string                  `json:"environment_id" yaml:"environment_id"`
	HostID        string                  `json:"host_id" yaml:"host_id"`
	Kind          string                  `json:"kind" yaml:"kind"`
	Purpose       string                  `json:"purpose" yaml:"purpose"`
	Description   string                  `json:"description" yaml:"description"`
	Workdir       string                  `json:"workdir" yaml:"workdir"`
	Repository    *RuntimeRepositoryInput `json:"repository" yaml:"repository"`
	Capabilities  []string                `json:"capabilities" yaml:"capabilities"`
	Supervisor    RuntimeSupervisorInput  `json:"supervisor" yaml:"supervisor"`
	Checks        []RuntimeCheckInput     `json:"checks" yaml:"checks"`
	Boundaries    RuntimeBoundariesInput  `json:"boundaries" yaml:"boundaries"`
}

type RuntimeRelationInput struct {
	Kind string `json:"kind" yaml:"kind"`
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

type RuntimeVerificationInput struct {
	VerifiedAt *string  `json:"verified_at,omitempty" yaml:"verified_at,omitempty"`
	VerifiedBy *string  `json:"verified_by,omitempty" yaml:"verified_by,omitempty"`
	StaleAfter *string  `json:"stale_after,omitempty" yaml:"stale_after,omitempty"`
	Evidence   []string `json:"evidence,omitempty" yaml:"evidence,omitempty" jsonschema:"Authority evidence references only. Each value must be a memory:// URI; write observed facts to handoff/progress first, then reference that resource here."`
}

type RuntimeManifestInput struct {
	SchemaVersion int                       `json:"schema_version" yaml:"schema_version"`
	ProjectID     string                    `json:"project_id" yaml:"project_id"`
	Environments  []RuntimeEnvironmentInput `json:"environments" yaml:"environments"`
	Relations     []RuntimeRelationInput    `json:"relations" yaml:"relations"`
	Verification  RuntimeVerificationInput  `json:"verification" yaml:"verification"`
}

type UpdateRuntimeArgs struct {
	ProjectID                string               `json:"project_id"`
	Runtime                  RuntimeManifestInput `json:"runtime"`
	ExpectedResourceRevision *string              `json:"expected_resource_revision,omitempty"`
	ExpectedIndexRevision    string               `json:"expected_index_revision"`
	ClientIdempotencyKey     string               `json:"client_idempotency_key,omitempty"`
}

type RecordRuntimeObservationArgs struct {
	ProjectID               string  `json:"project_id"`
	Observation             string  `json:"observation"`
	HostID                  string  `json:"host_id"`
	EnvironmentID           *string `json:"environment_id,omitempty"`
	RuntimeResourceRevision *string `json:"runtime_resource_revision,omitempty"`
	ClientIdempotencyKey    string  `json:"client_idempotency_key"`
}

type runtimeObservationRecord struct {
	SchemaVersion int            `json:"schema_version"`
	PayloadHash   string         `json:"payload_hash"`
	CallerKeyHash string         `json:"caller_key_hash"`
	Result        map[string]any `json:"result"`
}

func (s *Service) UpdateRuntime(args UpdateRuntimeArgs) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return preflight
	}
	if invalidLifecycleProjectID(args.ProjectID) || !validRevision(args.ExpectedIndexRevision) {
		return mutationError("validation_failed", "arguments", "invalid runtime update arguments")
	}
	if args.ExpectedResourceRevision != nil && !validRevision(*args.ExpectedResourceRevision) {
		return mutationError("validation_failed", "expected_resource_revision", "expected_resource_revision must be a SHA-256 revision")
	}
	if args.Runtime.ProjectID != args.ProjectID {
		return mutationError("validation_failed", "runtime_project_id", "runtime project_id does not match target")
	}
	args.Runtime = normalizeRuntimeManifestInput(args.Runtime)
	if payloadContainsHighConfidenceSecret(args.Runtime) {
		return mutationError("validation_failed", "content_secret", "runtime mutation contains high-confidence secret material")
	}

	payload := map[string]any{
		"operation": "update_runtime", "project_id": args.ProjectID, "runtime": args.Runtime,
		"expected_resource_revision": args.ExpectedResourceRevision, "expected_index_revision": args.ExpectedIndexRevision,
	}
	payloadHash, err := hashPayload(payload)
	if err != nil {
		return mutationError("mutation_failed", "payload", err.Error())
	}
	callerKeyHash := hashOptionalKey(args.ClientIdempotencyKey)
	legacyPayload := map[string]any{"runtime": args.Runtime, "expected_resource_revision": args.ExpectedResourceRevision, "expected_index_revision": args.ExpectedIndexRevision, "evidence": []EvidenceInput{}}
	if result, handled := s.callerKeyDispositionV2(callerKeyHash, args.ClientIdempotencyKey, payloadHash, args.ProjectID, "runtime", "update_runtime", legacyPayload); handled {
		return result
	}

	runtimeRaw, err := yaml.Marshal(args.Runtime)
	if err != nil {
		return mutationError("validation_failed", "runtime_yaml", err.Error())
	}
	runtimeRaw = canonicalManagedText(runtimeRaw)
	validation := runtimeview.ValidateManifestDetailed(runtimeRaw, args.ProjectID)
	if !validation.Valid {
		return runtimeValidationMutationError(validation)
	}

	_, indexDoc, indexRevision, err := s.readSchema4Index()
	if err != nil {
		return mutationError("validation_failed", "index", err.Error())
	}
	if args.ExpectedIndexRevision != indexRevision {
		return conflictWithCurrent("INDEX revision does not match current content", indexRevision)
	}
	project, ok := indexDoc.Projects[args.ProjectID]
	if !ok {
		return mutationError("validation_failed", "index_target", "project is absent from INDEX")
	}
	if project.Lifecycle != "active" {
		return mutationError("validation_failed", "project_archived", "runtime cannot be updated for an archived project")
	}

	runtimePath := filepath.ToSlash(filepath.Join("projects", args.ProjectID, "runtime.yaml"))
	changes := map[string]*[]byte{runtimePath: bytesPtr(runtimeRaw)}
	previousRevision := ""
	if project.RuntimeResource == nil {
		if args.ExpectedResourceRevision != nil {
			return conflictWithCurrent("runtime revision does not match current content", "")
		}
		if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(runtimePath))); err == nil {
			return mutationError("validation_failed", "runtime_registration", "unregistered runtime file already exists")
		} else if !os.IsNotExist(err) {
			return mutationError("mutation_failed", "runtime", err.Error())
		}
		value := runtimePath
		project.RuntimeResource = &value
		indexDoc.Projects[args.ProjectID] = project
		newIndex, err := yaml.Marshal(indexDoc)
		if err != nil {
			return mutationError("mutation_failed", "index", err.Error())
		}
		changes["INDEX.yaml"] = bytesPtr(newIndex)
	} else {
		if *project.RuntimeResource != runtimePath {
			return mutationError("validation_failed", "runtime_resource", "runtime resource path is invalid")
		}
		head, headErr := gitHead(s.root)
		if headErr != nil {
			return mutationError("mutation_failed", "git", headErr.Error())
		}
		currentRaw, err := readAuthorityAtCommit(s.root, head, runtimePath)
		if err != nil {
			return mutationError("validation_failed", "runtime_resource", "registered runtime resource is missing")
		}
		previousRevision = revision(currentRaw)
		if args.ExpectedResourceRevision == nil || *args.ExpectedResourceRevision != previousRevision {
			return conflictWithCurrent("runtime revision does not match current content", previousRevision)
		}
	}

	previousCommit, err := gitHead(s.root)
	if err != nil {
		return mutationError("mutation_failed", "git", err.Error())
	}
	spec := lifecycleResultSpec{
		Operation: "update_runtime", ProjectID: args.ProjectID, Role: "runtime",
		ResourceURI:      "memory://projects/" + args.ProjectID + "/runtime",
		PreviousRevision: previousRevision, NewRevision: revision(runtimeRaw), RelativePath: runtimePath,
	}
	return s.commitLifecycleChanges(spec, payloadHash, callerKeyHash, previousCommit, changes)
}

// normalizeRuntimeManifestInput keeps the public write contract ergonomic while
// preserving a single canonical runtime representation. Git checks can only
// target the environment repository, so callers do not need to repeat that
// invariant; accepted shorthand is expanded before hashing and persistence.
func normalizeRuntimeManifestInput(input RuntimeManifestInput) RuntimeManifestInput {
	out := input
	out.Environments = append([]RuntimeEnvironmentInput(nil), input.Environments...)
	for i := range out.Environments {
		env := out.Environments[i]
		env.Checks = append([]RuntimeCheckInput(nil), env.Checks...)
		for j := range env.Checks {
			check := env.Checks[j]
			if (check.Kind == "git_status" || check.Kind == "git_head") && check.Target == nil {
				target := "repository"
				check.Target = &target
			}
			env.Checks[j] = check
		}
		out.Environments[i] = env
	}
	return out
}

func (s *Service) RecordRuntimeObservation(args RecordRuntimeObservationArgs) map[string]any {
	if preflight := s.writePreflight(); preflight != nil {
		return runtimeObservationFromMutationError(preflight, args)
	}
	if invalidLifecycleProjectID(args.ProjectID) || (args.Observation != "runtime_conflict" && args.Observation != "fallback_to_handoff") || !runtimeHostID(args.HostID) || strings.TrimSpace(args.ClientIdempotencyKey) == "" {
		return runtimeObservationError("validation_failed", "arguments", "invalid runtime observation arguments", args, nil)
	}
	if args.EnvironmentID != nil && (strings.TrimSpace(*args.EnvironmentID) == "" || len(*args.EnvironmentID) > 64) {
		return runtimeObservationError("validation_failed", "environment_id", "invalid environment_id", args, nil)
	}
	if args.RuntimeResourceRevision != nil && !validRevision(*args.RuntimeResourceRevision) {
		return runtimeObservationError("validation_failed", "runtime_resource_revision", "runtime_resource_revision must be a SHA-256 revision", args, nil)
	}
	_, indexDoc, _, err := s.readSchema4Index()
	if err != nil {
		return runtimeObservationError("validation_failed", "index", err.Error(), args, nil)
	}
	project, ok := indexDoc.Projects[args.ProjectID]
	if !ok {
		return runtimeObservationError("validation_failed", "index_target", "project is absent from INDEX", args, nil)
	}
	if project.Lifecycle != "active" {
		return runtimeObservationError("validation_failed", "project_archived", "runtime observation requires an active project", args, nil)
	}

	payload := map[string]any{
		"project_id": args.ProjectID, "observation": args.Observation, "host_id": args.HostID,
		"environment_id": args.EnvironmentID, "runtime_resource_revision": args.RuntimeResourceRevision,
	}
	payloadHash, err := hashPayload(payload)
	if err != nil {
		return runtimeObservationError("mutation_failed", "payload", err.Error(), args, nil)
	}
	callerKeyHash := hashText(args.ClientIdempotencyKey)
	path := filepath.Join(s.workRoot, "runtime-observations", callerKeyHash+".json")
	if raw, err := os.ReadFile(path); err == nil {
		var record runtimeObservationRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			return runtimeObservationError("mutation_failed", "observation_record", err.Error(), args, nil)
		}
		if record.PayloadHash != payloadHash {
			return runtimeObservationError("validation_failed", "idempotency_key", "client_idempotency_key was already used for a different runtime observation", args, nil)
		}
		out := cloneMap(record.Result)
		out["status"] = "already_recorded"
		out["idempotent_replay"] = true
		return out
	} else if !os.IsNotExist(err) {
		return runtimeObservationError("mutation_failed", "observation_record", err.Error(), args, nil)
	}

	var currentRevision *string
	if project.RuntimeResource != nil {
		head, headErr := gitHead(s.root)
		if headErr != nil {
			return runtimeObservationError("mutation_failed", "git", headErr.Error(), args, nil)
		}
		raw, err := readAuthorityAtCommit(s.root, head, *project.RuntimeResource)
		if err != nil {
			return runtimeObservationError("validation_failed", "runtime_resource", "registered runtime resource is missing", args, nil)
		}
		rev := revision(raw)
		currentRevision = &rev
		if args.RuntimeResourceRevision != nil && *args.RuntimeResourceRevision != rev {
			return runtimeObservationError("conflict", "", "runtime revision changed before observation was recorded", args, currentRevision)
		}
		if args.Observation == "runtime_conflict" {
			if args.RuntimeResourceRevision == nil {
				return runtimeObservationError("validation_failed", "runtime_resource_revision", "runtime_conflict requires runtime_resource_revision", args, nil)
			}
			reg := runtimeview.Registration{ProjectID: args.ProjectID, Lifecycle: "active", RuntimeResource: *project.RuntimeResource}
			head, _ := gitHead(s.root)
			view := runtimeview.New(s.root, head, map[string]runtimeview.Registration{args.ProjectID: reg})
			query := runtimeview.QueryArgs{ProjectID: args.ProjectID, HostID: args.HostID, Detail: "summary"}
			if args.EnvironmentID != nil {
				query.EnvironmentID = *args.EnvironmentID
			}
			checked := view.ProjectRuntime(query)
			if checked["status"] != "ok" {
				out := runtimeObservationError("validation_failed", "runtime_scope", "runtime observation scope is not registered", args, nil)
				out["available_hosts"] = checked["available_hosts"]
				return out
			}
		}
	} else if args.Observation == "runtime_conflict" {
		return runtimeObservationError("validation_failed", "runtime_missing", "runtime_conflict requires a registered runtime resource", args, nil)
	}

	result := runtimeObservationBase(args)
	result["status"] = "recorded"
	result["caller_key_hash"] = callerKeyHash
	result["idempotent_replay"] = false
	result["audit_state"] = "recorded"
	record := runtimeObservationRecord{SchemaVersion: 1, PayloadHash: payloadHash, CallerKeyHash: callerKeyHash, Result: cloneMap(result)}
	if err := writeJSONDurable(path, record); err != nil {
		return runtimeObservationError("mutation_failed", "observation_record", err.Error(), args, nil)
	}
	return result
}

func runtimeValidationMutationError(report runtimeview.ValidationReport) map[string]any {
	if len(report.Errors) == 0 {
		return mutationError("validation_failed", "runtime", "runtime validation failed")
	}
	first := report.Errors[0]
	result := mutationError("validation_failed", first.Code, first.Message)
	result["validation_errors"] = report.Errors
	return result
}

func runtimeHostID(value string) bool { return value == "local" || value == "arm" || value == "amd" }

func runtimeObservationBase(args RecordRuntimeObservationArgs) map[string]any {
	var env, rev any
	if args.EnvironmentID != nil {
		env = *args.EnvironmentID
	}
	if args.RuntimeResourceRevision != nil {
		rev = *args.RuntimeResourceRevision
	}
	return map[string]any{
		"status": nil, "code": nil, "message": nil, "rule": nil, "current_revision": nil,
		"project_id": args.ProjectID, "observation": args.Observation, "host_id": args.HostID,
		"environment_id": env, "runtime_resource_revision": rev, "available_hosts": nil,
		"caller_key_hash": nil, "idempotent_replay": nil, "audit_state": nil,
	}
}

func runtimeObservationError(code, rule, message string, args RecordRuntimeObservationArgs, current *string) map[string]any {
	out := runtimeObservationBase(args)
	out["status"] = "error"
	out["code"] = code
	out["message"] = message
	if rule != "" {
		out["rule"] = rule
	}
	if current != nil {
		out["current_revision"] = *current
	}
	// Frozen output does not echo possibly stale revision/environment/host on conflict.
	if code == "conflict" {
		out["environment_id"] = nil
		out["host_id"] = nil
		out["runtime_resource_revision"] = nil
	}
	return out
}

func runtimeObservationFromMutationError(in map[string]any, args RecordRuntimeObservationArgs) map[string]any {
	code, _ := in["code"].(string)
	rule, _ := in["rule"].(string)
	message, _ := in["message"].(string)
	return runtimeObservationError(code, rule, message, args, nil)
}
