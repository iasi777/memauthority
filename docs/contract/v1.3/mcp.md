# Public MCP contract — 1.3.0

The server implementation identity is `MemAuthority` / `1.3.0`. `mcp-tools.json` records the complete runtime-enabled tool definitions; `mcp-resource-templates.json` records the optional runtime resource template. Runtime-free startup intentionally exposes a smaller subset.

The default server capability instructions, when runtime metadata is not enabled, are exactly:

> MemAuthority provides project-scoped long-term memory and cross-conversation continuity. Use it when prior project state is needed and is not already available in the current context. It can find and read durable memory and update validated Git-backed Authority. Authority writes must use MemAuthority mutation tools, not machine or filesystem tools.

When optional runtime metadata is enabled, the instructions are exactly:

> MemAuthority provides project-scoped long-term memory and cross-conversation continuity. Use it when prior project state is needed and is not already available in the current context. It can find and read durable memory and update validated Git-backed Authority. Authority writes must use MemAuthority mutation tools, not machine or filesystem tools. Optional declarative runtime metadata is enabled; it describes stored topology and does not prove live runtime health.

These instructions are capability guidance, not a mandatory invocation sequence. Context comes first. There is no contract requiring a fixed `route -> search -> read` workflow.

## Default runtime-free surface

A Vault with no registered `runtime_resource`, started without runtime enablement, exposes exactly 13 tools:

`memory_append_progress`, `memory_archive_project`, `memory_create_project`, `memory_mark_verified`, `memory_migrate_lifecycle`, `memory_read`, `memory_record_pitfall`, `memory_restore_project`, `memory_route`, `memory_search`, `memory_status`, `memory_update_handoff`, `memory_update_sections`.

It exposes no runtime resource template. This is the recommended surface for ordinary single-machine use and for onboarding/legacy-memory import unless runtime topology is genuinely useful.

## Runtime-enabled surface

Runtime capability is enabled by `--runtime-enabled`, preferred `MEMAUTHORITY_RUNTIME_ENABLED=1`, legacy `VM_RUNTIME_ENABLED=1`, or automatically when the admitted Vault already contains any registered runtime resource.

The runtime-enabled surface contains the 13 default tools plus:

`memory_project_runtime`, `memory_record_runtime_observation`, `memory_update_runtime`.

The runtime-enabled surface contains one resource template: `memory://projects/{project_id}/runtime`, named `memory_project_runtime_resource`, MIME type `application/yaml`.

`host_id` is not a closed maintainer vocabulary. It is optional user-defined identity matching `^[a-z0-9][a-z0-9._-]{0,63}$`. A single runtime environment may omit `environment_id`; it canonicalizes to `default`. Multi-environment manifests require explicit unique environment IDs. `runtime-closed-values.json` contains only the remaining validator-owned closed vocabularies.

Each tool has a human-oriented title plus a purpose-specific description. Nested closed values intentionally remain validator-owned rather than SDK typed enums so multiple independent runtime findings can be reported together.

Runtime is declarative stored metadata, not a memory role and not proof of live runtime health. The legacy migration comparator helpers `memory_history`, `memory_diff`, `memory_list_todos`, and `memory_doctor` are not public MCP tools.

Tool names, titles, descriptions, input/output schemas and annotations in `mcp-tools.json`, the conditional exposure rules above, the optional resource-template schema, the implementation identity/version, and the two instruction variants above are the v1.3.0 compatibility surface represented by this snapshot.
