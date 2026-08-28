# Public `v-memory` CLI contract — 1.1.0

The public executable is `v-memory`. It exposes `init`, `validate`, `serve`, `version`, and `--version`. Historical/internal `v-memory-mcp` is not part of the public CLI contract.

## `v-memory init <path>`

An explicit target is required. Init creates a dedicated empty Git Vault only when the target is absent or an empty real directory. It refuses symlink/non-directory targets, non-empty unknown directories, and creation of a nested Git repository inside an existing worktree. There is no destructive `--force` path.

A new Vault contains Git metadata, `INDEX.yaml` with `schema_version: 4` and `projects: {}`, and `.gitattributes` fixing Markdown/YAML to LF. It creates branch `main` and mechanical commit `v-memory: initialize empty Vault` using controlled identity `V-Memory MCP <v-memory@v-memory.invalid>`. It does not create demo projects, templates, runtime state or semantic memory.

Re-running init on an already-valid Git-backed V-Memory Vault is idempotent and does not move HEAD or rewrite files.

## `v-memory validate [--json] [path]`

The path defaults to the current directory and must itself be a Git repository root with a commit. A dirty worktree is allowed. The command reuses the public shared Vault validator and is read-only.

Human success output is `VALID`. JSON output is schema-1 validation JSON. Exit status is `0` for valid, `1` for invalid/refusal, and `2` for usage/environment/internal execution failure. There is no public `--fix` behavior.

## `v-memory serve --vault <path> --state-dir <path> [options]`

Both paths are required. The state directory is runtime-private and must resolve outside Vault Authority. The Vault must be valid, committed and clean at managed admission.

Default transport is `stdio`. Authority mutation tools are disabled by default and require `--write-enabled`. Fencing can be required with `--require-primary`, a positive `--primary-epoch`, and non-empty external `--node-id-file`.

Deployment-specific path, host, OAuth credential, node and domain values are not compatibility constants.

## Version discovery

`v-memory version` and `v-memory --version` are both public version-discovery interfaces. In v1.1.0 they exit 0 and print exactly `v-memory 1.1.0` plus newline. Extra arguments are usage errors (exit 2). See `VERSION-DECISION.md` for the SemVer compatibility policy.
