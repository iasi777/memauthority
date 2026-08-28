# V-Memory v1.2 Public Contract

This directory contains the versioned public contract for V-Memory `1.2.0`.

It covers:

- `vault-format.md`: Vault/Authority and validation behavior;
- `cli.md`: public `v-memory` CLI behavior;
- `mcp.md`, `mcp-tools.json`, `mcp-resource-templates.json`: MCP capability, tool, optional runtime exposure, and resource-template surface;
- `runtime-closed-values.json`: runtime values that remain validator-owned closed vocabularies;
- `managed-runtime.md`: managed snapshot, mutation, fencing, and durability semantics;
- `transport-auth.md`: stdio/HTTP/OAuth behavior and safety boundaries;
- `structured-errors.md`: externally observable refusal and error classes;
- `VERSION-DECISION.md`: release-visible version and compatibility policy;
- `MANIFEST.json`: frozen source binding and normative file hashes.

The contract describes externally observable behavior. Deployment-specific paths, credentials, hostnames, private runtime state layout, and maintainer infrastructure are not compatibility constants unless explicitly stated.

Runtime metadata is optional and progressive in v1.2. Vaults without a registered runtime do not expose runtime tools/resources by default. Existing Vaults with registered runtime remain auto-enabled, and new advanced setups may opt in explicitly with `--runtime-enabled`.

`MANIFEST.json` binds this contract to the reviewed public source baseline. The `v1.2.0` tag identifies the commit that records the freeze manifest.
