# MemAuthority v1.3 Version Policy

The release-visible policy for this contract is:

- release/tag identity: `v1.3.0`; semantic software version: `1.3.0`;
- product and canonical repository identity: `MemAuthority` / `iasi777/memauthority`;
- the v1.x Go module identity remains `github.com/iasi777/v-memory` for backward-compatible module/install semantics; the GitHub repository rename is intentionally not treated as a Go module rename;
- MCP `initialize` reports `serverInfo.name = MemAuthority` and `serverInfo.version = 1.3.0`;
- `memauthority version` and `memauthority --version` exit 0 and print exactly `memauthority 1.3.0` plus newline;
- the compatibility executable `v-memory version` and `v-memory --version` exit 0 and print exactly `v-memory 1.3.0` plus newline;
- extra arguments to either version form are usage errors (exit 2);
- compatibility follows Semantic Versioning: breaking public changes require a major version, backward-compatible public additions use a minor version, and backward-compatible fixes use a patch version.

`v1.3.0` is a backward-compatible minor release over `v1.2.0`. It changes the public product/repository brand and adds the `memauthority` executable plus preferred `MEMAUTHORITY_*` environment aliases, while preserving the v1.x module identity, legacy executable, Vault format, durable markers, mutation semantics, tool names and runtime behavior.
