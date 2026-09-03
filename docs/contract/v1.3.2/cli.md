# Public MemAuthority CLI contract — 1.3.2

The primary public executable is `memauthority`. The compatibility executable `v-memory` exposes the same command set and remains supported throughout the v1.x line. Both expose `init`, `validate`, `serve`, `version`, and `--version`. Historical/internal `v-memory-mcp` is not part of the public CLI contract.

## `memauthority init <path>` / `v-memory init <path>`

An explicit target is required. Init creates a dedicated empty Git Vault only when the target is absent or an empty real directory. It refuses symlink/non-directory targets, non-empty unknown directories, and creation of a nested Git repository inside an existing worktree. There is no destructive `--force` path.

A new Vault contains Git metadata, `INDEX.yaml` with `schema_version: 4` and `projects: {}`, and `.gitattributes` fixing Markdown/YAML to LF. It creates branch `main`. To preserve durable v1 compatibility, the mechanical initial commit remains `v-memory: initialize empty Vault` using controlled identity `V-Memory MCP <v-memory@v-memory.invalid>`. These persisted Git strings are compatibility data, not the current product brand. It does not create demo projects, templates, runtime state or semantic memory.

Re-running init on an already-valid Git-backed Vault is idempotent and does not move HEAD or rewrite files. The primary executable reports `MemAuthority Vault` in human-facing init output; the compatibility executable retains `V-Memory Vault` wording.

## `validate [--json] [path]`

The path defaults to the current directory and must itself be a Git repository root with a commit. A dirty worktree is allowed. The command reuses the public shared Vault validator and is read-only.

Human success output is `VALID`. JSON output is schema-1 validation JSON. Exit status is `0` for valid, `1` for invalid/refusal, and `2` for usage/environment/internal execution failure. There is no public `--fix` behavior.

## `serve --vault <path> --state-dir <path> [options]`

Both paths are required. The state directory is runtime-private and must resolve outside Vault Authority. The Vault must be valid, committed and clean at managed admission.

Default transport is `stdio`. Authority mutation tools are disabled by default and require `--write-enabled`. Fencing can be required with `--require-primary`, a positive `--primary-epoch`, and non-empty external `--node-id-file`.

Optional declarative runtime tools/resources are not exposed by default when the admitted Vault has no registered `runtime_resource`. `--runtime-enabled`, `MEMAUTHORITY_RUNTIME_ENABLED=1`, or legacy `VM_RUNTIME_ENABLED=1` opts a runtime-free Vault into that capability. If any project already has a registered runtime resource at startup, runtime capability exposure is enabled automatically.

### Environment-variable compatibility

For renamed configuration variables, the `MEMAUTHORITY_*` form is preferred and the v1 legacy `VM_*` form remains accepted. If both names are present, the `MEMAUTHORITY_*` value wins, including when explicitly set to the empty string. The compatibility pairs are:

- `MEMAUTHORITY_OAUTH_ISSUER_URL` / `VM_OAUTH_ISSUER_URL`;
- `MEMAUTHORITY_OAUTH_CLIENT_ID` / `VM_OAUTH_CLIENT_ID`;
- `MEMAUTHORITY_OAUTH_CLIENT_SECRET_FILE` / `VM_OAUTH_CLIENT_SECRET_FILE`;
- `MEMAUTHORITY_OAUTH_REDIRECT_URIS` / `VM_OAUTH_REDIRECT_URIS`;
- `MEMAUTHORITY_OAUTH_USERNAME` / `VM_OAUTH_USERNAME`;
- `MEMAUTHORITY_OAUTH_PASSWORD_FILE` / `VM_OAUTH_PASSWORD_FILE`;
- `MEMAUTHORITY_OAUTH_STATE_DB` / `VM_OAUTH_STATE_DB`;
- `MEMAUTHORITY_AUTH_TOKEN_FILE` / `VM_AUTH_TOKEN_FILE`;
- `MEMAUTHORITY_ALLOWED_HOSTS` / `VM_ALLOWED_HOSTS`;
- `MEMAUTHORITY_ALLOWED_ORIGINS` / `VM_ALLOWED_ORIGINS`;
- `MEMAUTHORITY_RUNTIME_ENABLED` / `VM_RUNTIME_ENABLED`;
- `MEMAUTHORITY_WRITE_SOURCE` / `VM_WRITE_SOURCE`;
- `MEMAUTHORITY_REQUIRE_PRIMARY` / `VM_REQUIRE_PRIMARY`;
- `MEMAUTHORITY_NODE_ID_FILE` / `VM_NODE_ID_FILE`.

Deployment-specific path, host, OAuth credential, node and domain values are not compatibility constants. Existing production issuer domains, deployment gateway paths and custom header names may remain deployment-local legacy identifiers and do not define the product brand.

## Version discovery

`memauthority version` and `memauthority --version` print exactly `memauthority 1.3.2` plus newline. The compatibility forms `v-memory version` and `v-memory --version` print exactly `v-memory 1.3.2` plus newline. Extra arguments are usage errors (exit 2). See `VERSION-DECISION.md` for the SemVer compatibility policy.
