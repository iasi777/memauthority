# V-Memory v1.2 Version Policy

The release-visible policy for this contract is:

- release/tag identity: `v1.2.0`; semantic software version: `1.2.0`;
- MCP `initialize` reports `serverInfo.name = V-Memory` and `serverInfo.version = 1.2.0`;
- `v-memory version` exits 0 and prints exactly `v-memory 1.2.0` plus newline;
- `v-memory --version` exits 0 and prints exactly `v-memory 1.2.0` plus newline;
- extra arguments to either version form are usage errors (exit 2);
- compatibility follows Semantic Versioning: breaking public changes require a major version, backward-compatible public additions use a minor version, and backward-compatible fixes use a patch version.

`v1.2.0` is a backward-compatible minor release over the first public `v1.1.0` line. Existing Vaults with registered runtime metadata retain runtime capability exposure automatically; runtime-free Vaults receive the smaller default MCP surface.
