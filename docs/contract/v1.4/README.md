# v1.4 mutation contract (unreleased)

This directory tracks the working MCP tool contract. Released snapshots,
including v1.3.2, remain unchanged; the implementation version is not a release
announcement. Resource templates are unchanged from v1.3.2.

- Ordinary handoff H2 sections support explicit insert and delete in addition to
  replace and append. Insert preserves literal headings, including slashes.
- Existing calls retain their semantics. Replace and append never create missing
  sections. Protected verification still requires `memory_mark_verified`.
- Optional TODOs remain top-level checklist items under `已知问题 / 待办`.
  Missing/empty sections are valid. H2 deletion reports `removed_checklist_items`,
  including checked items; deletion does not assert completion. The count is
  persisted with the transaction and returned on idempotent replay.
- Reads return resource-wide `sections` (exact heading, allowed operations,
  protection, dedicated tool), role-level `allowed_operations`, and
  `section_creation_hint`, all describing the returned resource revision.
  Role-level operations do not override section protection.
- Section rendering errors return the same metadata and current revision.
  Invalid arguments and preflight failures may occur before a resource is read.
- Tool schemas enumerate roles and operation names. Role-specific permissions
  remain validated by the mutation implementation.

Regenerate the tool snapshot after intentional changes:

```bash
UPDATE_MCP_SNAPSHOT=1 go test ./internal/mcpserver -run TestCurrentRuntimeEnabledContractSnapshot
```
