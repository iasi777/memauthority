# V-Memory v1.1 Version Policy

The release-visible policy for this contract is:

- release/tag identity: `v1.1.0`; semantic software version: `1.1.0`;
- MCP `initialize` reports `serverInfo.name = V-Memory` and `serverInfo.version = 1.1.0`;
- `v-memory version` exits 0 and prints exactly `v-memory 1.1.0` plus newline;
- `v-memory --version` exits 0 and prints exactly `v-memory 1.1.0` plus newline;
- extra arguments to either version form are usage errors (exit 2);
- compatibility follows Semantic Versioning: breaking public changes require a major version, backward-compatible public additions use a minor version, and backward-compatible fixes use a patch version.

`v1.1.0` is the first public source release in the rebuilt public repository. Earlier private development and compatibility-freeze records are not part of the public repository history.
