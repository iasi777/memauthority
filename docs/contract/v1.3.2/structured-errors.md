# Structured errors and refusal behavior — 1.3.2

Public CLI exit classes are stable at the command boundary: `0` success, `1` invalid/refused operation, `2` usage/environment/internal execution failure where documented by the CLI contract.

Validation JSON uses schema version 1 and deterministic findings. Findings expose code/path/line/field/message as applicable; secret findings never include matched secret values.

Managed Authority mutation is fail-closed. Publicly relevant structured refusal classes include CAS/resource revision conflict, snapshot drift, primary/fencing refusal, protected/invalid mutation, duplicate entry/idempotency handling, and validation/secret-scan refusal. Successful mutation is not acknowledged before the durable transaction reaches its defined committed/finalized result.

## Aggregated runtime validation

When a fresh `memory_update_runtime` request reaches static manifest validation and the manifest is invalid, the response remains compatible with the existing mutation error envelope:

- `status = "error"`;
- `code = "validation_failed"`;
- top-level `rule` and `message` identify the first deterministic finding;
- additive `validation_errors` contains all independent repairable findings in deterministic order.

Each `validation_errors[]` item has `code` and `message`, with `path`, `field`, and `line` present when applicable. Parent discriminator failures suppress dependent child-field noise instead of emitting misleading cascades. Secret material is never copied into finding messages.

This aggregation changes diagnosis, not the accepted runtime language. Closed values are defined by `runtime-closed-values.json`. The static validator runs before fresh INDEX/CAS reads, so an invalid runtime manifest is reported as invalid input even when the caller also supplied a stale revision. Invalid static input does not move HEAD or create mutation journal/operation state.

Physical journal/operation record JSON layouts are private. The observable guarantees are deterministic completion/recovery, idempotent replay behavior, no acknowledged write loss, and structured errors rather than silent partial success.
