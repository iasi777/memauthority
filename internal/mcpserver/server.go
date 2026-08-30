// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/iasi777/memauthority/internal/gitview"
	"github.com/iasi777/memauthority/internal/oauthserver"
	"github.com/iasi777/memauthority/internal/pathguard"
	"github.com/iasi777/memauthority/internal/runtimeview"
	"github.com/iasi777/memauthority/internal/vaultread"
	"github.com/iasi777/memauthority/internal/vaultvalidate"
	"github.com/iasi777/memauthority/internal/vaultwrite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const implementationName = "MemAuthority"
const ImplementationVersion = "1.3.0-dev"

// Keep Layer 0 discovery compact; contract tests enforce a 512-character ceiling.
func serverInstructions(runtimeEnabled bool) string {
	base := "MemAuthority provides project-scoped long-term memory and cross-conversation continuity. Use it when prior project state is needed and is not already available in the current context. It can find and read durable memory and update validated Git-backed Authority. Authority writes must use MemAuthority mutation tools, not machine or filesystem tools."
	if !runtimeEnabled {
		return base
	}
	return base + " Optional declarative runtime metadata is enabled; it describes stored topology and does not prove live runtime health."
}

type OpenOptions struct {
	OAuth          *oauthserver.Config
	WriteEnabled   bool
	WriteSource    string
	RequirePrimary bool
	PrimaryEpoch   int
	LocalNodeID    string
	AllowedHosts   []string
	AllowedOrigins []string
	RuntimeEnabled bool

	// ManagedAdmission is used by the public serve entrypoint. It requires a
	// structurally valid, clean committed Vault and an external state directory.
	ManagedAdmission bool

	// Test hooks are internal conformance seams, not product configuration.
	TestBeforeViewPublish       func(previousHead, newHead string)
	TestMutationCriticalSection func()
}

type App struct {
	viewMu         sync.RWMutex
	mutationMu     sync.Mutex
	vault          *vaultread.Service
	runtime        *runtimeview.Service
	ownedHead      string
	viewError      string
	writer         *vaultwrite.Service
	server         *mcp.Server
	oauth          *oauthserver.Server
	vaultRoot      string
	workRoot       string
	writeEnabled   bool
	runtimeEnabled bool
	allowedHosts   []string
	allowedOrigins []string

	testBeforeViewPublish       func(previousHead, newHead string)
	testMutationCriticalSection func()
}

type RouteArgs struct {
	Query string `json:"query" jsonschema:"Project routing query. Matching is exact after Unicode NFC normalization and case folding against project IDs and aliases; ambiguous and archived matches are reported explicitly."`
}

func Open(vaultRoot, workRoot string) (*App, error) {
	return OpenWithOptions(vaultRoot, workRoot, OpenOptions{})
}

func OpenWithOptions(vaultRoot, workRoot string, options OpenOptions) (*App, error) {
	app := &App{
		vaultRoot: vaultRoot, workRoot: workRoot, writeEnabled: options.WriteEnabled, runtimeEnabled: options.RuntimeEnabled,
		allowedHosts: normalizePolicyValues(options.AllowedHosts, true), allowedOrigins: normalizePolicyValues(options.AllowedOrigins, false),
		testBeforeViewPublish: options.TestBeforeViewPublish, testMutationCriticalSection: options.TestMutationCriticalSection,
	}
	if options.ManagedAdmission {
		if strings.TrimSpace(workRoot) == "" {
			return nil, fmt.Errorf("managed serve requires external state directory")
		}
		if inside, err := isWithin(vaultRoot, workRoot); err != nil {
			return nil, fmt.Errorf("validate state directory: %w", err)
		} else if inside {
			return nil, fmt.Errorf("state directory must be outside Vault Authority")
		}
		if err := managedAdmissionCheck(vaultRoot); err != nil {
			return nil, err
		}
	}
	if workRoot != "" {
		source := strings.TrimSpace(options.WriteSource)
		if source == "" {
			source = "mcp"
		}
		writer, err := vaultwrite.New(vaultRoot, vaultwrite.Config{
			WorkRoot: workRoot, WriteEnabled: options.WriteEnabled, WriteSource: source,
			RequirePrimary: options.RequirePrimary, PrimaryEpoch: options.PrimaryEpoch, LocalNodeID: options.LocalNodeID,
		})
		if err != nil {
			return nil, err
		}
		app.writer = writer
	}
	if app.writer == nil || !app.writer.LegacyLifecycleReady() {
		if err := app.refreshViewsAt(""); err != nil {
			return nil, err
		}
	}
	if options.OAuth != nil {
		if inside, err := isWithin(vaultRoot, options.OAuth.StateDB); err != nil {
			app.Close()
			return nil, fmt.Errorf("validate OAuth state path: %w", err)
		} else if inside {
			app.Close()
			return nil, fmt.Errorf("OAuth state DB must be outside Vault Authority")
		}
		if inside, err := isWithin(vaultRoot, options.OAuth.PasswordFile); err != nil {
			app.Close()
			return nil, fmt.Errorf("validate OAuth password path: %w", err)
		} else if inside {
			app.Close()
			return nil, fmt.Errorf("OAuth password file must be outside Vault Authority")
		}
		for label, credentialPath := range map[string]string{"OAuth client secret": options.OAuth.ClientSecretFile, "deployment auth token": options.OAuth.StaticTokenFile} {
			if strings.TrimSpace(credentialPath) == "" {
				continue
			}
			if inside, err := isWithin(vaultRoot, credentialPath); err != nil {
				app.Close()
				return nil, fmt.Errorf("validate %s path: %w", label, err)
			} else if inside {
				app.Close()
				return nil, fmt.Errorf("%s file must be outside Vault Authority", label)
			}
		}
		authServer, err := oauthserver.Open(*options.OAuth)
		if err != nil {
			app.Close()
			return nil, err
		}
		app.oauth = authServer
	}
	if !app.runtimeEnabled && app.hasRegisteredRuntime() {
		app.runtimeEnabled = true
	}
	if options.ManagedAdmission {
		status := app.managedStatus()
		if status["snapshot_state"] != "current" || status["workspace_clean"] != true {
			app.Close()
			return nil, fmt.Errorf("managed serve admission lost a clean committed snapshot")
		}
	}
	app.server = app.newServer()
	return app, nil
}

func managedAdmissionCheck(vaultRoot string) error {
	if err := vaultvalidate.ValidateAuthorityRoot(vaultRoot); err != nil {
		return fmt.Errorf("Vault validation failed: %w", err)
	}
	if _, err := exec.Command("git", "-C", vaultRoot, "rev-parse", "--verify", "HEAD^{commit}").Output(); err != nil {
		return fmt.Errorf("managed serve requires a committed Git Vault")
	}
	out, err := exec.Command("git", "-C", vaultRoot, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return fmt.Errorf("inspect managed Vault worktree: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("managed serve requires a clean Vault worktree")
	}
	return nil
}

func isWithin(root, candidate string) (bool, error) {
	return pathguard.Contains(root, candidate)
}

func (a *App) Close() error {
	var errs []error
	if a.oauth != nil {
		errs = append(errs, a.oauth.Close())
	}
	a.viewMu.Lock()
	vault := a.vault
	a.vault = nil
	a.runtime = nil
	a.viewMu.Unlock()
	if vault != nil {
		errs = append(errs, vault.Close())
	}
	return errors.Join(errs...)
}

func (a *App) Server() *mcp.Server { return a.server }

func (a *App) HTTPHandler() http.Handler {
	if a.writeEnabled && a.oauth == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "write-enabled HTTP requires OAuth", http.StatusServiceUnavailable)
		})
	}
	opts := &mcp.StreamableHTTPOptions{JSONResponse: true}
	if len(a.allowedHosts) > 0 {
		// An explicit exact Host allowlist supersedes the SDK localhost-only Host
		// heuristic. The outer transport policy remains fail-closed.
		opts.DisableLocalhostProtection = true
	}
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return a.server }, opts)
	var handler http.Handler = streamable
	if a.oauth != nil {
		mux := http.NewServeMux()
		mux.Handle("/mcp", a.oauth.Protect(streamable))
		mux.Handle("/.well-known/oauth-protected-resource/mcp", a.oauth.ProtectedResourceMetadataHandler())
		mux.Handle("/.well-known/oauth-authorization-server", a.oauth.AuthorizationServerMetadataHandler())
		mux.Handle("/authorize", a.oauth.AuthorizeHandler())
		mux.Handle("/oauth/login", a.oauth.LoginHandler())
		mux.Handle("/token", a.oauth.TokenHandler())
		mux.Handle("/revoke", a.oauth.RevokeHandler())
		handler = mux
	}
	return transportPolicy(handler, a.allowedHosts, a.allowedOrigins)
}

func normalizePolicyValues(values []string, lower bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func transportPolicy(next http.Handler, allowedHosts, allowedOrigins []string) http.Handler {
	hosts := map[string]bool{}
	for _, host := range allowedHosts {
		hosts[strings.ToLower(host)] = true
	}
	origins := map[string]bool{}
	for _, origin := range allowedOrigins {
		origins[origin] = true
	}
	if len(hosts) == 0 && len(origins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(hosts) > 0 && !hosts[strings.ToLower(strings.TrimSpace(r.Host))] {
			http.Error(w, "Forbidden: invalid Host header", http.StatusForbidden)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && len(origins) > 0 && !origins[origin] {
			http.Error(w, "Forbidden: invalid Origin header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) RunStdio(ctx context.Context) error {
	return a.server.Run(ctx, &mcp.StdioTransport{})
}

func boolPtr(v bool) *bool { return &v }

func mutationAnnotations(idempotent, destructive bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: false, DestructiveHint: boolPtr(destructive), IdempotentHint: idempotent, OpenWorldHint: boolPtr(false),
	}
}

func statusAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}
}

func markVerifiedInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[vaultwrite.MarkVerifiedArgs](nil)
	if err != nil {
		panic(fmt.Errorf("infer memory_mark_verified input schema: %w", err))
	}
	level := schema.Properties["verification_level"]
	if level == nil {
		panic("memory_mark_verified input schema lacks verification_level")
	}
	level.Enum = []any{"local", "runtime"}
	return schema
}

func runtimeUpdateInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[vaultwrite.UpdateRuntimeArgs](nil)
	if err != nil {
		panic(fmt.Errorf("infer memory_update_runtime input schema: %w", err))
	}
	values := runtimeview.ClosedValues()
	join := func(xs []string) string { return strings.Join(xs, ", ") }

	setSchemaDescription(schema, "project_id", "Registered active MemAuthority project id. runtime.project_id must match this value.")
	setSchemaDescription(schema, "expected_index_revision", "Required INDEX CAS revision in sha256:<64 hex> form. Stale values return conflict after static runtime validation passes.")
	setSchemaDescription(schema, "expected_resource_revision", "Runtime resource CAS revision in sha256:<64 hex> form when replacing an existing runtime. Omit only when creating the project's first registered runtime.")
	setSchemaDescription(schema, "client_idempotency_key", "Optional caller idempotency key. Reusing the same key with the same payload replays the committed result; reusing it for a different payload conflicts.")

	runtimeSchema := mustSchemaProperty(schema, "runtime")
	setSchemaDescription(runtimeSchema, "schema_version", "Runtime schema version. The only supported value is 1.")
	setSchemaDescription(runtimeSchema, "project_id", "Project id stored in the runtime manifest; must equal the top-level project_id target.")
	setSchemaDescription(runtimeSchema, "environments", "One to 64 declarative runtime environments. This metadata describes stored topology and does not execute or prove live checks.")
	setSchemaDescription(runtimeSchema, "relations", "Zero to 128 directed relations between declared environment_id values.")
	setSchemaDescription(runtimeSchema, "verification", "Either empty/unverified, or complete with verified_at, verified_by, stale_after, and one or more memory:// Authority evidence URIs.")

	env := mustSchemaItems(mustSchemaProperty(runtimeSchema, "environments"), "runtime.environments")
	setSchemaDescription(env, "environment_id", "Optional for a single environment; omission is canonicalized to default. Multi-environment manifests must provide a unique stable id matching ^[a-z0-9][a-z0-9._-]{0,63}$.")
	setSchemaDescription(env, "host_id", "Optional user-defined execution host identity matching ^[a-z0-9][a-z0-9._-]{0,63}$. Use it only when grouping multiple environments by machine or logical host is useful.")
	setSchemaDescription(env, "kind", "Environment shape. Supported values: "+join(values.EnvironmentKinds)+".")
	setSchemaDescription(env, "purpose", "Environment purpose. Supported values: "+join(values.Purposes)+".")
	setSchemaDescription(env, "description", "Short non-empty single-line description, at most 500 characters.")
	setSchemaDescription(env, "workdir", "Absolute environment working directory without . or .. traversal segments.")
	setSchemaDescription(env, "repository", "Optional repository metadata. When present, path and branch are required; path must be absolute and remote is optional.")
	setSchemaDescription(env, "capabilities", "Zero to 16 unique capabilities. Supported values: "+join(values.Capabilities)+".")
	setSchemaDescription(env, "supervisor", "Supervisor declaration. Required companion fields depend on supervisor.kind; only fields for that kind are accepted.")
	setSchemaDescription(env, "checks", "Zero to 32 declarative check specifications. Checks are stored only; memory_update_runtime does not execute them.")
	setSchemaDescription(env, "boundaries", "Mutation/deployment boundary flags. production_writer and mutation_requires_explicit_authorization are required booleans; single_writer is optional.")

	supervisor := mustSchemaProperty(env, "supervisor")
	setSchemaDescription(supervisor, "kind", "Supervisor discriminator. Supported values: "+join(values.SupervisorKinds)+". none requires no companion fields; systemd_user/systemd_system require units; docker_compose requires compose_file and services; docker requires containers; process requires process_name and pid_file.")
	setSchemaDescription(supervisor, "units", "Required non-empty unit list only for systemd_user or systemd_system supervisors.")
	setSchemaDescription(supervisor, "compose_file", "Required absolute compose file path only for docker_compose supervisors.")
	setSchemaDescription(supervisor, "services", "Required non-empty service list only for docker_compose supervisors.")
	setSchemaDescription(supervisor, "containers", "Required non-empty container list only for docker supervisors.")
	setSchemaDescription(supervisor, "process_name", "Required non-empty process name only for process supervisors.")
	setSchemaDescription(supervisor, "pid_file", "Required absolute pid file path only for process supervisors.")

	check := mustSchemaItems(mustSchemaProperty(env, "checks"), "runtime.environments[].checks")
	setSchemaDescription(check, "kind", "Check discriminator. Supported values: "+join(values.CheckKinds)+". Companion fields depend on kind.")
	setSchemaDescription(check, "scope", "Required for systemd_units and failed_units; supported values: user, system.")
	setSchemaDescription(check, "units", "Required non-empty unit list for systemd_units.")
	setSchemaDescription(check, "compose_file", "Required absolute compose file path for docker_compose_services.")
	setSchemaDescription(check, "services", "Required non-empty service list for docker_compose_services.")
	setSchemaDescription(check, "path", "Required absolute path for file_exists and directory_exists.")

	relation := mustSchemaItems(mustSchemaProperty(runtimeSchema, "relations"), "runtime.relations")
	setSchemaDescription(relation, "kind", "Relation kind. Supported values: "+join(values.RelationKinds)+".")
	setSchemaDescription(relation, "from", "Source environment_id; must reference a declared environment and differ from to.")
	setSchemaDescription(relation, "to", "Destination environment_id; must reference a declared environment and differ from from.")

	verification := mustSchemaProperty(runtimeSchema, "verification")
	setSchemaDescription(verification, "verified_at", "RFC3339 verification timestamp. If verification is populated, all verification fields are required.")
	setSchemaDescription(verification, "verified_by", "Non-empty verifier identity. If verification is populated, all verification fields are required.")
	setSchemaDescription(verification, "stale_after", "Positive day duration in PnD form, for example P30D.")
	return schema
}

func setSchemaDescription(schema *jsonschema.Schema, property, description string) {
	mustSchemaProperty(schema, property).Description = description
}

func mustSchemaProperty(schema *jsonschema.Schema, property string) *jsonschema.Schema {
	if schema == nil || schema.Properties == nil || schema.Properties[property] == nil {
		panic(fmt.Sprintf("runtime input schema lacks property %s", property))
	}
	return schema.Properties[property]
}

func mustSchemaItems(schema *jsonschema.Schema, path string) *jsonschema.Schema {
	if schema == nil || schema.Items == nil {
		panic(fmt.Sprintf("runtime input schema lacks items for %s", path))
	}
	return schema.Items
}

func titledAnnotations(title string, annotations *mcp.ToolAnnotations) *mcp.ToolAnnotations {
	if annotations == nil {
		return &mcp.ToolAnnotations{Title: title}
	}
	copy := *annotations
	copy.Title = title
	return &copy
}

func runtimeUpdateToolDescription() string {
	values := runtimeview.ClosedValues()
	return fmt.Sprintf("Use this only when optional runtime metadata is enabled and deployment/work-environment topology is worth retaining. A single environment may omit environment_id (canonicalized to default), host_id is optional and user-defined, and environment kinds %s, supervisors %s, and checks %s remain validated vocabularies. It uses runtime and INDEX revision/CAS safeguards and does not execute deployments or live health checks.", strings.Join(values.EnvironmentKinds, "/"), strings.Join(values.SupervisorKinds, "/"), strings.Join(values.CheckKinds, "/"))
}

func (a *App) hasRegisteredRuntime() bool {
	a.viewMu.RLock()
	defer a.viewMu.RUnlock()
	if a.vault == nil {
		return false
	}
	for _, project := range a.vault.Projects() {
		if project.RuntimeResource != nil && strings.TrimSpace(*project.RuntimeResource) != "" {
			return true
		}
	}
	return false
}

func (a *App) newServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: implementationName, Version: ImplementationVersion}, &mcp.ServerOptions{Instructions: serverInstructions(a.runtimeEnabled)})
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "memory_route", Description: "Use this when the target MemAuthority project id is uncertain and you have a project name, alias, or repository identity. It resolves exact NFC-normalized, Unicode-casefolded registered project ids or aliases; do not use it when the project id or resource URI is already known.", Annotations: titledAnnotations("Resolve memory project", ro)},
		func(_ context.Context, _ *mcp.CallToolRequest, in RouteArgs) (*mcp.CallToolResult, map[string]any, error) {
			if in.Query == "" {
				return nil, nil, fmt.Errorf("query is required")
			}
			a.viewMu.RLock()
			defer a.viewMu.RUnlock()
			if a.vault == nil {
				return nil, nil, fmt.Errorf("read side unavailable until lifecycle migration")
			}
			return nil, a.vault.Route(in.Query), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_read", Description: "Use this to read a known handoff, rules, progress, or pitfalls memory resource by URI, optionally by heading or line range. Prefer it directly when the resource URI is already known; it does not read runtime resources. Eligible handoff reads also return checklist_items for top-level todos under 已知问题 / 待办.", Annotations: titledAnnotations("Read memory resource", ro)},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultread.ReadArgs) (*mcp.CallToolResult, map[string]any, error) {
			if in.URI == "" {
				return nil, nil, fmt.Errorf("uri is required")
			}
			a.viewMu.RLock()
			defer a.viewMu.RUnlock()
			if a.vault == nil {
				return nil, nil, fmt.Errorf("read side unavailable until lifecycle migration")
			}
			return nil, a.vault.Read(in), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_search", Description: "Use this to locate relevant indexed sections inside a known project when the exact memory resource or section is unknown. It returns up to 50 coordinate-like hits for selective reading, not final proof or complete context; truncated=true means additional matches exist. It does not resolve uncertain project identity.", Annotations: titledAnnotations("Search project memory", ro)},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultread.SearchArgs) (*mcp.CallToolResult, map[string]any, error) {
			if in.Query == "" {
				return nil, nil, fmt.Errorf("query is required")
			}
			a.viewMu.RLock()
			defer a.viewMu.RUnlock()
			if a.vault == nil {
				return nil, nil, fmt.Errorf("read side unavailable until lifecycle migration")
			}
			return nil, a.vault.Search(in), nil
		})
	if a.runtimeEnabled {
		mcp.AddTool(s, &mcp.Tool{Name: "memory_project_runtime", Description: "Use this to read stored declarative runtime location, environment, repository, supervisor, check, and boundary metadata for a project. It does not execute checks, inspect the MemAuthority service itself, or prove current live deployment health.", Annotations: titledAnnotations("Read declarative runtime", ro)},
			func(_ context.Context, _ *mcp.CallToolRequest, in runtimeview.QueryArgs) (*mcp.CallToolResult, map[string]any, error) {
				if in.ProjectID == "" {
					return nil, nil, fmt.Errorf("project_id is required")
				}
				a.viewMu.RLock()
				defer a.viewMu.RUnlock()
				if a.runtime == nil {
					return nil, nil, fmt.Errorf("runtime view unavailable until lifecycle migration")
				}
				return nil, a.runtime.ProjectRuntime(in), nil
			})
	}

	a.addMutationTools(s)

	if a.runtimeEnabled {
		s.AddResourceTemplate(&mcp.ResourceTemplate{
			URITemplate: "memory://projects/{project_id}/runtime",
			Name:        "memory_project_runtime_resource",
			Description: "Validated raw runtime manifest resource; distinct from the four memory roles.",
			MIMEType:    "application/yaml",
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			uri := req.Params.URI
			projectID, ok := runtimeProjectID(uri)
			a.viewMu.RLock()
			defer a.viewMu.RUnlock()
			if !ok || a.runtime == nil {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			actualURI, content, err := a.runtime.ReadRawResource(projectID)
			if err != nil {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: actualURI, MIMEType: "application/yaml", Text: content}}}, nil
		})
	}
	return s
}

func (a *App) addMutationTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "memory_append_progress", Description: "Append a dated typed progress entry through the durable Authority transaction without replacing existing progress history.", Annotations: titledAnnotations("Append project progress", mutationAnnotations(true, false))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.AppendProgressArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.AppendProgress(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_update_handoff", Description: "Use this for direct replacement of allowed canonical handoff sections with resource CAS. Prefer memory_update_sections when you need ordered append/insert/delete operations, checklist item changes, or edits outside the handoff role.", Annotations: titledAnnotations("Update handoff summary", mutationAnnotations(false, true))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.UpdateHandoffArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.UpdateHandoff(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_update_sections", Description: "Use this for ordered typed section edits across supported memory roles with resource CAS. H2 operations may replace, append, insert, or delete according to role; handoff also supports checklist_remove and checklist_set_checked using item_ref values returned by memory_read. Prefer memory_update_handoff for simple whole-section handoff replacement.", Annotations: titledAnnotations("Edit memory sections", mutationAnnotations(false, true))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.UpdateSectionsArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.UpdateSections(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_mark_verified", Description: "Record protected local or runtime verification state through the controlled mutation path after the required evidence has actually been checked.", InputSchema: markVerifiedInputSchema(), Annotations: titledAnnotations("Mark memory verified", mutationAnnotations(false, true))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.MarkVerifiedArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.MarkVerified(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_record_pitfall", Description: "Idempotently append a typed reusable pitfall entry to project memory without replacing existing pitfalls.", Annotations: titledAnnotations("Record project pitfall", mutationAnnotations(true, false))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.RecordPitfallArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.RecordPitfall(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_create_project", Description: "Create a new active MemAuthority project with a program-owned canonical handoff scaffold; no Vault template file is required.", Annotations: titledAnnotations("Create memory project", mutationAnnotations(false, false))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.CreateProjectArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.CreateProject(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_archive_project", Description: "Archive one MemAuthority project in current-HEAD lifecycle metadata while preserving its Authority history.", Annotations: titledAnnotations("Archive memory project", mutationAnnotations(true, true))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.ArchiveProjectArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.ArchiveProject(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_restore_project", Description: "Restore one archived MemAuthority project to active lifecycle state through the controlled mutation path.", Annotations: titledAnnotations("Restore memory project", mutationAnnotations(true, true))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.RestoreProjectArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.RestoreProject(in) }), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "memory_migrate_lifecycle", Description: "Perform the one-time legacy MemAuthority lifecycle migration through the transaction kernel; this is a migration operation, not normal project editing.", Annotations: titledAnnotations("Migrate memory lifecycle", mutationAnnotations(false, true))},
		func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.MigrateLifecycleArgs) (*mcp.CallToolResult, map[string]any, error) {
			return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.MigrateLifecycle(in) }), nil
		})
	if a.runtimeEnabled {
		mcp.AddTool(s, &mcp.Tool{Name: "memory_update_runtime", Description: runtimeUpdateToolDescription(), InputSchema: runtimeUpdateInputSchema(), Annotations: titledAnnotations("Update declarative runtime", mutationAnnotations(false, true))},
			func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.UpdateRuntimeArgs) (*mcp.CallToolResult, map[string]any, error) {
				return nil, a.callAuthorityMutation(func(w *vaultwrite.Service) map[string]any { return w.UpdateRuntime(in) }), nil
			})
		mcp.AddTool(s, &mcp.Tool{Name: "memory_record_runtime_observation", Description: "Idempotently record a typed runtime conflict or completed fallback as control evidence without mutating project Authority.", Annotations: titledAnnotations("Record runtime observation", mutationAnnotations(true, false))},
			func(_ context.Context, _ *mcp.CallToolRequest, in vaultwrite.RecordRuntimeObservationArgs) (*mcp.CallToolResult, map[string]any, error) {
				return nil, a.callControlMutation(func(w *vaultwrite.Service) map[string]any { return w.RecordRuntimeObservation(in) }), nil
			})
	}

	mcp.AddTool(s, &mcp.Tool{Name: "memory_status", Description: "Use this to inspect the MemAuthority service and Authority workspace itself: Git head, owned snapshot, projection, write gate, fencing, journal, and recovery state. It does not report a project's deployment location or live production service health.", Annotations: titledAnnotations("Inspect memory service status", statusAnnotations())},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
			a.mutationMu.Lock()
			defer a.mutationMu.Unlock()
			if a.writer == nil {
				return nil, a.statusWithoutWriter(), nil
			}
			return nil, a.managedStatus(), nil
		})
}

func (a *App) callAuthorityMutation(call func(*vaultwrite.Service) map[string]any) map[string]any {
	a.mutationMu.Lock()
	defer a.mutationMu.Unlock()
	if a.testMutationCriticalSection != nil {
		a.testMutationCriticalSection()
	}
	if a.writer == nil {
		return writeDisabledResult()
	}
	if gate := a.snapshotMutationGate(); gate != nil {
		return gate
	}
	result := call(a.writer)
	if result["status"] != "committed" {
		return result
	}
	newHead, _ := result["new_commit"].(string)
	if strings.TrimSpace(newHead) == "" {
		return result
	}
	a.viewMu.RLock()
	previousHead := a.ownedHead
	a.viewMu.RUnlock()
	if a.testBeforeViewPublish != nil {
		a.testBeforeViewPublish(previousHead, newHead)
	}
	if err := a.refreshViewsAt(newHead); err != nil {
		a.viewMu.Lock()
		a.viewError = err.Error()
		a.viewMu.Unlock()
	}
	return result
}

func (a *App) callControlMutation(call func(*vaultwrite.Service) map[string]any) map[string]any {
	a.mutationMu.Lock()
	defer a.mutationMu.Unlock()
	if a.writer == nil {
		return writeDisabledResult()
	}
	return call(a.writer)
}

func (a *App) snapshotMutationGate() map[string]any {
	a.viewMu.RLock()
	owned, ownedIndex := a.ownedHead, ""
	if a.vault != nil {
		ownedIndex = a.vault.IndexHead()
	}
	a.viewMu.RUnlock()
	if owned == "" {
		return nil
	} // legacy lifecycle migration owns no schema-4 read view yet.
	status := a.writer.Status(ownedIndex)
	head, _ := status["git_head"].(string)
	clean, _ := status["workspace_clean"].(bool)
	if clean && head == owned && ownedIndex == owned {
		return nil
	}
	return map[string]any{
		"status": "error", "code": "read_only", "rule": "snapshot_drift",
		"message":     "managed Authority snapshot drift detected; restart/reopen is required before mutation",
		"dirty_paths": status["dirty_paths"], "owned_head": owned, "git_head": head, "audit_state": "recorded",
	}
}

func (a *App) managedStatus() map[string]any {
	if a.writer == nil {
		return a.statusWithoutWriter()
	}
	a.viewMu.RLock()
	owned, ownedIndex, viewErr := a.ownedHead, "", a.viewError
	if a.vault != nil {
		ownedIndex = a.vault.IndexHead()
	}
	a.viewMu.RUnlock()
	status := a.writer.Status(ownedIndex)
	head, _ := status["git_head"].(string)
	clean, _ := status["workspace_clean"].(bool)
	drift := make([]string, 0, 4)
	if !clean {
		drift = append(drift, "workspace_dirty")
	}
	if owned == "" {
		drift = append(drift, "owned_head_unavailable")
	} else if head != owned {
		drift = append(drift, "head_moved")
	}
	if owned != "" && ownedIndex != owned {
		drift = append(drift, "owned_projection_mismatch")
	}
	if viewErr != "" {
		drift = append(drift, "view_publish_failed")
	}
	state := "current"
	if len(drift) > 0 {
		state = "degraded"
	}
	status["owned_head"] = owned
	status["owned_index_head"] = ownedIndex
	status["snapshot_state"] = state
	status["runtime_tools_enabled"] = a.runtimeEnabled
	status["snapshot_drift"] = drift
	if viewErr == "" {
		status["snapshot_error"] = nil
	} else {
		status["snapshot_error"] = viewErr
	}
	return status
}

func (a *App) refreshViewsAt(expectedHead string) error {
	head, err := gitview.ResolveCommit(a.vaultRoot, expectedHead)
	if err != nil {
		return err
	}
	opts := vaultread.OpenOptions{SourceCommit: head}
	if a.workRoot != "" {
		opts.ProjectionPath = filepath.Join(a.workRoot, "owned-views", head+".sqlite")
	}
	vault, err := vaultread.OpenWithOptions(a.vaultRoot, opts)
	if err != nil {
		return err
	}
	if vault.SourceCommit() != head || (opts.ProjectionPath != "" && vault.IndexHead() != head) {
		_ = vault.Close()
		return fmt.Errorf("owned read projection is not bound to expected commit")
	}
	registrations := make(map[string]runtimeview.Registration)
	for _, project := range vault.Projects() {
		reg := runtimeview.Registration{ProjectID: project.ID, Lifecycle: project.Lifecycle}
		if project.RuntimeResource != nil {
			reg.RuntimeResource = *project.RuntimeResource
		}
		if project.ArchivedAt != nil {
			reg.ArchivedAt = *project.ArchivedAt
		}
		if project.ArchiveReason != nil {
			reg.ArchiveReason = *project.ArchiveReason
		}
		registrations[project.ID] = reg
	}
	runtime := runtimeview.New(a.vaultRoot, head, registrations)

	a.viewMu.Lock()
	old := a.vault
	oldHead := a.ownedHead
	a.vault, a.runtime, a.ownedHead, a.viewError = vault, runtime, head, ""
	a.viewMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	if a.workRoot != "" && oldHead != "" && oldHead != head {
		for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
			_ = os.Remove(filepath.Join(a.workRoot, "owned-views", oldHead+".sqlite") + suffix)
		}
	}
	return nil
}

func writeDisabledResult() map[string]any {
	return map[string]any{
		"status": "error", "code": "write_disabled", "message": "write operations are disabled", "rule": nil, "audit_state": "recorded",
		"all_rolled_back": nil, "archive_reason": nil, "archived_at": nil, "available_sections": nil, "backup_state": nil,
		"caller_key_hash": nil, "content_hash": nil, "current_revision": nil, "evidence_count": nil, "failed_operation": nil,
		"failed_operation_index": nil, "idempotent_replay": nil, "incremental_validation": nil, "index_state": nil,
		"new_commit": nil, "new_revision": nil, "operation_id": nil, "previous_commit": nil, "previous_revision": nil,
		"project_id": nil, "resource_uri": nil, "runtime_status": nil, "source": nil,
	}
}

func (a *App) statusWithoutWriter() map[string]any {
	a.viewMu.RLock()
	owned, indexHead, viewErr := a.ownedHead, "", a.viewError
	if a.vault != nil {
		indexHead = a.vault.IndexHead()
	}
	a.viewMu.RUnlock()
	head, headErr := gitHeadObserved(a.vaultRoot)
	clean, dirty, statusErr := gitWorktreeObserved(a.vaultRoot)
	drift := make([]string, 0, 4)
	if statusErr != nil || headErr != nil {
		drift = append(drift, "git_unavailable")
		clean = false
	}
	if !clean {
		drift = append(drift, "workspace_dirty")
	}
	if owned != "" && head != "" && head != owned {
		drift = append(drift, "head_moved")
	}
	if viewErr != "" {
		drift = append(drift, "view_publish_failed")
	}
	snapshotState := "current"
	if len(drift) > 0 {
		snapshotState = "degraded"
	}
	indexState := "unconfigured"
	if indexHead != "" {
		indexState = "current"
	}
	var snapshotError any
	if viewErr != "" {
		snapshotError = viewErr
	}
	return map[string]any{
		"workspace_clean": clean, "dirty_paths": dirty, "read_only": true, "write_enabled": false,
		"write_mode": "disabled", "git_head": head, "index_head": indexHead, "index_state": indexState, "write_journal_count": 0,
		"primary_active": false, "primary": map[string]any{"state": "missing", "node_id": nil, "epoch": nil},
		"owned_head": owned, "owned_index_head": indexHead, "snapshot_state": snapshotState, "snapshot_drift": drift, "snapshot_error": snapshotError,
	}
}

func gitHeadObserved(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD^{commit}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitWorktreeObserved(root string) (bool, []string, error) {
	out, err := exec.Command("git", "-C", root, "-c", "core.quotePath=false", "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return false, []string{}, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return true, []string{}, nil
	}
	paths := make([]string, 0)
	for _, line := range strings.Split(text, "\n") {
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

func runtimeProjectID(uri string) (string, bool) {
	const prefix = "memory://projects/"
	const suffix = "/runtime"
	if len(uri) <= len(prefix)+len(suffix) || uri[:len(prefix)] != prefix || uri[len(uri)-len(suffix):] != suffix {
		return "", false
	}
	id := uri[len(prefix) : len(uri)-len(suffix)]
	if id == "" {
		return "", false
	}
	for _, r := range id {
		if r == '/' || r == '%' || r == '\\' {
			return "", false
		}
	}
	return id, true
}
