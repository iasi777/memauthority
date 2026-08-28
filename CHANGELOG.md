# Changelog

## Unreleased

No unreleased product changes yet.

## v1.2.0

- Added native Linux, macOS, and Windows CI validation; fixed Windows directory durability handling and made managed revisions consistently bind to Git Authority bytes instead of checkout-specific line-ending transformations.
- Made optional declarative runtime metadata progressive: Vaults without registered runtime metadata no longer expose runtime tools/resources by default. Existing Vaults with runtime metadata remain auto-enabled, and new advanced setups can opt in with `--runtime-enabled`.
- Removed maintainer-specific `local` / `arm` / `amd` host IDs from the public runtime vocabulary. `host_id` is now optional and user-defined.
- Made single-environment runtime input ergonomic: omitted `environment_id` canonicalizes to `default`; multi-environment manifests still require explicit unique IDs.
- Updated onboarding and migration guidance to recommend leaving runtime metadata disabled unless durable environment/deployment topology is actually useful.

The current versioned public contract is maintained under `docs/contract/v1.2/`.

## v1.1.0

- First public source release of V-Memory.
- Added demand-driven MCP capability discovery with compact server instructions, tool titles, and clearer point-of-use descriptions without imposing a fixed recall ritual.
- Added handoff checklist mutation through `memory_update_sections` using stable `item_ref` values returned by `memory_read`.
- Exposed the already-enforced `local` / `runtime` verification-level closed set in the MCP schema.
- Added dependency-aware aggregated runtime validation through `validation_errors[]` while preserving the existing top-level mutation error fields.
- Run static runtime validation before INDEX/CAS reads so invalid input cannot create mutation state or be masked by stale revisions.
- Added English-primary, Chinese-paired user documentation, a runnable sanitized sample Vault, and public security guidance.

The versioned public contract is maintained under `docs/contract/v1.1/`.
