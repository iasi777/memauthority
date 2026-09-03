# MemAuthority public contract — v1.3.2

This directory is the frozen public compatibility contract for MemAuthority `v1.3.2`, a backward-compatible packaging patch over v1.3.1. The patch adds MCP Bundle (MCPB) distribution metadata for the Official MCP Registry; it does not change the Vault format, MCP tool surface, CLI command surface, transport/auth behavior, or v1.x Go module identity.

Normative files:

- `vault-format.md`: Git-backed Vault layout, revisions, lifecycle and durable marker compatibility;
- `cli.md`: primary `memauthority` CLI plus the `v-memory` compatibility executable and environment-variable aliases;
- `mcp.md`, `mcp-tools.json`, `mcp-resource-templates.json`: MCP identity, capability, tool, optional runtime exposure and resource-template surface;
- `managed-runtime.md`: transaction, admission, recovery, fencing and runtime capability behavior;
- `structured-errors.md`: structured validation, refusal and aggregated runtime error behavior;
- `transport-auth.md`: stdio/HTTP exposure and OAuth security behavior;
- `runtime-closed-values.json`: exact validator-owned runtime closed vocabularies;
- `VERSION-DECISION.md`: release-visible identity and compatibility policy;
- `MANIFEST.json`: frozen source binding and normative file hashes.

All earlier released contract directories remain immutable historical snapshots and must not be rewritten in place.

`MANIFEST.json` binds this contract to the reviewed public source baseline. The `v1.3.2` tag identifies the commit that records the freeze manifest.
