# Changelog

## Unreleased

No unreleased product changes yet.

## v1.1.0

- First public source release of V-Memory.
- Added demand-driven MCP capability discovery with compact server instructions, tool titles, and clearer point-of-use descriptions without imposing a fixed recall ritual.
- Added handoff checklist mutation through `memory_update_sections` using stable `item_ref` values returned by `memory_read`.
- Exposed the already-enforced `local` / `runtime` verification-level closed set in the MCP schema.
- Added dependency-aware aggregated runtime validation through `validation_errors[]` while preserving the existing top-level mutation error fields.
- Run static runtime validation before INDEX/CAS reads so invalid input cannot create mutation state or be masked by stale revisions.
- Added English-primary, Chinese-paired user documentation, a runnable sanitized sample Vault, and public security guidance.

The versioned public contract is maintained under `docs/contract/v1.1/`.
