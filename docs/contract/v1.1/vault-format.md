# Public Vault and Authority contract — 1.1.0

## Authority boundary

A V-Memory Vault is a Git repository. At the current Git HEAD, V-Memory-defined Markdown/YAML/marker paths are Authority. Support content such as README or LICENSE is not memory Authority merely because it is stored in the repository.

SQLite projection/cache state is rebuildable and not part of the public Vault file format. Durable journal/operation/transaction state is runtime-private control state: its physical schema is not public, but its durability/recovery semantics are part of the managed-runtime contract.

The four memory roles are fixed: `handoff`, `rules`, `progress`, `pitfalls`. Runtime is a separate subsystem, not a fifth memory role.

## INDEX and project IDs

`INDEX.yaml` uses schema version 4 and contains the registered project map. `projects: {}` is a valid empty public bootstrap Vault.

Project IDs match `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`. Original case is preserved. IDs that collide under ASCII case folding are invalid. Reserved identifiers are case-insensitive: `projects`, `migrations`, `PRIMARY`, and Windows device names `CON`, `PRN`, `AUX`, `NUL`, `COM1`..`COM9`, `LPT1`..`LPT9`.

Every registered project requires `handoff`. `rules`, `progress`, and `pitfalls` are optional. Existing role files require frontmatter fields `项目`, `创建`, `最后更新`, `最后核验`; `说明` is required for handoff/rules and, when present for progress/pitfalls, must be non-empty.

A registered project directory contains only the four canonical role filenames. An ordinary unregistered directory containing a canonical role filename is invalid. Support namespaces beginning with `_` or `.` are not inferred as projects.

`projects/<id>/runtime.yaml` is the only runtime resource location. Active registered runtime must exist and validate; active unregistered runtime is invalid. A structurally valid archived dormant runtime file is allowed for restore compatibility.

## Validation

Validation is deterministic, read-only and non-semantic. It checks Authority structure/safety, portable IDs, role frontmatter/root headings, date ordering and cross-file synchronization, fixed namespaces, runtime registration, UTF-8/source limits, path/file/symlink safety, and the shared high-confidence secret scanner.

Validation does not require a clean worktree, does not consult the current time, does not score memory quality/staleness, does not emit suggestions, and does not auto-fix content. CRLF/CR line endings and non-NFC text alone are not validation errors; managed writers emit canonical NFC/LF text.

Validation errors are deterministically ordered by path, line, code and field. Secret findings reveal only safe metadata, never the secret value. Runtime manifest closed values are listed in `runtime-closed-values.json`; `structured-errors.md` defines the aggregated mutation diagnostic surface.

## Detached vs managed authoring

Detached authoring permits the Vault owner/Agent to directly edit the open Git/Markdown/YAML working tree. A commit makes those changes Authority. Direct detached authoring and managed runtime ownership do not operate concurrently.

After managed runtime admission, Authority mutation is typed and controlled through V-Memory mutation tools. Machine/filesystem tools must not directly modify managed Authority, even when the surrounding execution environment technically has enough operating-system privilege to do so. There is no raw file, shell or raw Git mutation API in V-Memory.
