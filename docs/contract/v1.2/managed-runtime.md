# Managed runtime contract — 1.2.0

Managed `serve` adopts one immutable `owned_head` after validating a clean committed Vault. Role reads, search projection, typed runtime view and raw runtime resource are attributable to that same commit; managed reads do not consume dirty worktree bytes.

In-process Authority mutation is serialized by one Vault-level critical section. Cross-process durability/safety remains protected by CAS, single-primary fencing, journal/recovery, path allowlists, protected sections, secret scanning and idempotency.

A successful mutation follows the durable transaction ordering: validate mutations, construct and validate candidate, persist/fsync prepared journal, Git CAS, materialize, index, finalize. A new managed read view is published only after the commit/materialize/index/finalize path completes. Readers already in flight may finish against the prior owned head; no request may observe a half-materialized tree or read/search commit mismatch.

For `memory_update_runtime`, idempotent replay disposition is checked before fresh static runtime validation. Fresh static runtime validation completes before INDEX read or CAS comparison. Invalid runtime input therefore cannot be masked by a stale INDEX revision and does not move HEAD or create transaction/journal mutation state.

External worktree dirtiness or Git HEAD movement does not silently advance `owned_head`. Managed status reports degraded snapshot state with deterministic drift reasons; Authority mutation fails closed with `snapshot_drift`. Reads may continue against the last valid owned snapshot with exact commit attribution. Restart/reopen from a clean valid Vault is the re-adoption path.

`transactions/`, `operations/`, `runtime-observations/`, and OAuth state when configured are durable control state. Read projections and deterministic temporary candidate/index files are rebuildable. Runtime state is outside Vault Authority.

Remote push/network success is not a transaction success condition.
