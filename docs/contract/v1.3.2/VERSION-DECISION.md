# MemAuthority v1.3.2 Version Policy

The release-visible policy for this contract is:

- release/tag identity: `v1.3.2`; semantic software version: `1.3.2`;
- product and canonical repository identity: `MemAuthority` / `iasi777/memauthority`;
- the v1.x Go module identity remains `github.com/iasi777/v-memory` for backward-compatible module/install semantics;
- MCP `initialize` reports `serverInfo.name = MemAuthority` and `serverInfo.version = 1.3.2`;
- `memauthority version` and `memauthority --version` exit 0 and print exactly `memauthority 1.3.2` plus newline;
- the compatibility executable `v-memory version` and `v-memory --version` exit 0 and print exactly `v-memory 1.3.2` plus newline;
- extra arguments to either version form are usage errors (exit 2);
- compatibility follows Semantic Versioning: breaking public changes require a major version, backward-compatible public additions use a minor version, and backward-compatible fixes use a patch version.

`v1.3.2` is a compatibility-preserving packaging patch over `v1.3.1`. It adds MCPB release artifacts and Official MCP Registry metadata. Public runtime behavior, MCP tools/resources, CLI behavior, Vault format, durable identifiers, transport/auth behavior, and environment-variable compatibility remain unchanged apart from the patch version string.
