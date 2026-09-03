# MCP Registry packaging

This directory contains the MCP Bundle manifest template used to prepare MemAuthority for the Official MCP Registry.

The v1.3.2 packaging candidate produces six native bundles:

- Windows x64 / ARM64;
- Linux x64 / ARM64;
- macOS x64 / ARM64.

Each MCPB contains one statically built `memauthority` binary plus the project and third-party license notices. The bundle asks the user to select a Vault and an external runtime-state directory. Write access is **off by default** and must be enabled explicitly in the bundle settings.

`tools/package-mcpb.sh` renders and validates the platform manifest with the pinned MCPB CLI, then creates a deterministic ZIP-compatible `.mcpb`. The deterministic archive step exists because MCPB CLI 2.1.2 writes the packaging time into ZIP metadata; the bundle contents are still validated and inspected with the official MCPB tooling.

`tools/serverjson` computes the SHA-256 of all six bundles and renders the root `server.json`. `.github/workflows/mcp-registry-package.yml` reproduces the packaging and validates `server.json` with `mcp-publisher` without authenticating or publishing anything.

The v1.3.2 release is not frozen or published until its source baseline and generated artifacts are reviewed.
