# Public MCP contract — 1.1.0

The server implementation identity is `V-Memory` / `1.1.0`. The exact public tool schemas are `mcp-tools.json`; the exact public resource-template schemas are `mcp-resource-templates.json`.

The server capability instructions are exactly:

> V-Memory provides project-scoped long-term memory and cross-conversation continuity. Use it when prior project state is needed and is not already available in the current context. It can find and read durable memory, update validated Git-backed Authority, and store declarative runtime metadata. Runtime metadata describes stored state; it does not prove live runtime health. Authority writes must use V-Memory mutation tools, not machine or filesystem tools.

These instructions are capability guidance, not a mandatory invocation sequence. Context comes first. `route`, `search`, `read`, `project_runtime`, and mutation tools are selected according to the task; there is no contract requiring a fixed `route -> search -> read` workflow.

The snapshot contains exactly 16 tools:

`memory_append_progress`, `memory_archive_project`, `memory_create_project`, `memory_mark_verified`, `memory_migrate_lifecycle`, `memory_project_runtime`, `memory_read`, `memory_record_pitfall`, `memory_record_runtime_observation`, `memory_restore_project`, `memory_route`, `memory_search`, `memory_status`, `memory_update_handoff`, `memory_update_runtime`, `memory_update_sections`.

Each tool has a human-oriented title plus a purpose-specific description. The `memory_update_runtime` selected-tool schema describes closed runtime values and discriminator-dependent companion fields at point of use. Nested closed values intentionally remain validator-owned rather than SDK typed enums so multiple independent runtime findings can be reported together. `runtime-closed-values.json` is the normative list of those closed value sets for this snapshot.

The snapshot contains one resource template: `memory://projects/{project_id}/runtime`, named `memory_project_runtime_resource`, MIME type `application/yaml`.

Runtime is declarative stored metadata, not a memory role and not proof of live runtime health. The legacy migration comparator helpers `memory_history`, `memory_diff`, `memory_list_todos`, and `memory_doctor` are not public MCP tools.

Tool names, titles, descriptions, input/output schemas and annotations in `mcp-tools.json`, the resource-template schema, the implementation version, and the server instructions above are the v1.1.0 compatibility surface represented by this snapshot.
