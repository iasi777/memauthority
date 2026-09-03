# MemAuthority Agent Guide

[中文](AGENT-GUIDE_ZH.md)

> Purpose: operating guidance for agents using MemAuthority.
> Exact tool schemas, Vault format, and refusal behavior are defined by the versioned public contract under [`contract/v1.3.2/`](contract/v1.3.2/).

This guide does not duplicate the full MCP schema. Once a Managed MCP connection is established, the agent already receives the current tool descriptions, input schemas, annotations, and resource metadata.

What follows covers the cross-tool principles that a single tool schema cannot express well, plus the authoring principles needed for detached bootstrap and legacy-memory migration.

---

## 1. Responsibility Boundaries

- **The user** makes a small number of high-level decisions: what deserves long-term retention, what should stay out of long-term memory, and whether important changes match the user's actual intent.
- **You, the agent** handle day-to-day execution: interpret, select, retrieve, compress, merge, update, retire, and migrate memory.
- **MemAuthority** provides the reliability layer: deterministic recall, controlled mutation, revision safety, and Git-backed Authority.

> **MemAuthority constrains state transitions, not intelligence.**

You decide what the content means and whether it should change based on the real task. Do not treat conversation history as long-term memory by default.

---

## 2. The Retention Test

Only consider long-term retention when a future agent knowing something would provide at least one clear benefit:

- change future behavior;
- restore necessary project state;
- avoid substantial re-reasoning;
- prevent a recurring mistake.

Use four destinations:

- **Context** — needed only for the current task;
- **Memory** — future agents should still know it;
- **TODO** — worth resuming later, but not worth current attention;
- **Forget** — temporary, private, duplicate, obsolete, already consumed, or low-value material.

> **Do not maximize how much is remembered. Maximize the value of future recall.**

---

## 3. The Four Memory Roles

### `handoff`

Keep the smallest working set needed to take over the project now.

Avoid:

- complete history;
- a second README;
- long-term accumulation of completed states.

### `rules`

Keep durable decisions and constraints that future work must still follow.

Avoid:

- debate transcripts;
- superseded rules;
- temporary preferences.

### `progress`

Keep milestone progress that still has future value.

It is the lowest-friction recording path, not a permanent operation log.

### `pitfalls`

Keep failure modes and avoidance patterns that may recur.

Do not record every one-off error.

### TODO

A TODO is a top-level checklist item under the canonical handoff H2 heading `Known Issues / TODO` (the literal heading used by the Vault format).

It represents deferred intent, not deadline / owner / schedule management.

### Write Maintainable Sections

Each section should ideally represent one topic that can be recalled, become stale, and be updated independently.

Prefer:

- a heading that states the topic directly;
- a first paragraph that states the current conclusion or state;
- only the reasoning needed to preserve future action implications;
- content that can be understood without the original conversation;
- separate sections for unrelated decisions;
- no full correction history inside active memory.

> **The heading says what this is; the first sentence says what is true now.**

---

## 4. Progressive Recall

Do not mechanically read MemAuthority at the start of every task, and do not load the whole Vault or all four roles at once.

First decide whether the current context is already sufficient. If it is, do not call MemAuthority. When durable project state is actually needed, choose the smallest action that matches what is already known:

- project ID / alias uncertain -> `memory_route`;
- exact URI, role, or section known -> direct `memory_read`;
- project known but location unknown -> project-scoped `memory_search`;
- overall takeover state needed -> `handoff` is usually the highest-value first resource;
- cross-project search only when the task explicitly needs cross-project experience;
- expand further only when the task requires it.

> **Context first. Recall on demand.**

### Search Results Are Coordinates, Not Context

A `memory_search` hit is a candidate location, not an instruction to inject the result into context automatically.

For selected results, prefer the returned:

- `resource_uri`;
- `line_start` / `line_end`;
- `resource_revision`.

Recommended flow:

```text
search
  -> inspect candidates
  -> select relevant hit
  -> memory_read(uri + line range + expected_resource_revision)
```

If the revision conflicts, read the current resource again and reason from the current state.

Do not mechanically reuse the full search heading path as a `memory_read` heading selector.

A cursor only means more content exists; it does not mean all remaining content must be read.

---

## 5. Managed Write Principles

While Managed ownership is active, Authority mutations must use MemAuthority mutation tools. Do not create a second Authority write path by modifying the Vault through generic machine or filesystem tools.

v1 is designed for a single-user, single-writer workflow. Different agents or clients can take turns using the same Authority, but it is not a collaborative database for simultaneous team editing.

For large detached authoring work, stop Managed ownership first, edit directly, validate, review, commit, and then restart the Managed service.

### 5.1 Read Before Updating Existing Content

CAS-protected mutations should use the current revision returned by `memory_read`.

On conflict:

1. do not retry blindly;
2. read the latest resource;
3. compare the current memory with the new facts from this task;
4. decide whether to merge, replace, abandon, or ask the user;
5. write again from the current state.

MemAuthority detects the stale view. You resolve the semantics.

### 5.2 Role Mutations

Prefer the descriptions and input schemas exposed by the current MCP connection. Do not guess operations from an old prompt, an old guide, or another deployment instance.

Across releases, keep these principles:

- `handoff`: maintain the current takeover state; update the protected verification section only through `memory_mark_verified`;
- `rules`: maintain currently valid standing rules rather than appending correction history;
- `progress`: prefer `memory_append_progress` for normal new progress;
- `pitfalls`: prefer `memory_record_pitfall` for normal new pitfalls;
- TODO: use only checklist operations advertised by the current schema; do not invent operations that the service does not expose.

If a mutation field or operation is critical to the task, confirm it against both the public contract for that release and the schema returned by the current runtime.

### 5.3 Idempotency

When retrying the same logical mutation because delivery is uncertain, reuse a stable `client_idempotency_key`.

Never reuse the same key for a different payload.

---

## 6. Preserve Current Truth, Not Complete History

Active memory should converge continuously.

Prefer to:

- replace obsolete state and rules;
- remove active memory that no longer has future value;
- keep only enough rationale to prevent future incorrect behavior;
- let Git history preserve how Authority changed over time.

> **Git preserves what happened; active MemAuthority preserves what future agents still need to know now.**

Removing active memory does not physically erase older Git commits.

---

## 7. When the User Only Says “Record This Task”

This is a normal low-effort workflow.

Use a conservative default instead of promoting everything that “looks useful” into a long-term role.

Default behavior:

1. prefer `memory_append_progress`;
2. record the task outcome;
3. record meaningful current-state changes;
4. keep only what is still needed to continue later;
5. omit material with weak future value.

Do not save:

- chat transcripts;
- hidden reasoning;
- pleasantries;
- operation-by-operation attempts;
- failures with no future value;
- duplicate facts already present;
- clearly temporary, obsolete, private, or speculative material.

Do not write something into `rules` merely because it sounds important.

Only when long-term significance is clear, or the user explicitly asks, should you:

- update `rules`;
- update `handoff`;
- record a `pitfall`.

Expected safety model:

> **Conservative agent capture + deterministic MemAuthority write safety + later convergence and cleanup.**

`progress` is the low-friction entry point. Later real work can distill stable conclusions into other roles or remove progress that has lost value.

---

## 8. Maintenance After a Task

The user can simply say:

```text
Review the MemAuthority memory actually used in this task. Update it from what just happened or was verified, and remove anything obsolete.
```

Equivalent natural-language instructions are fine. You should:

1. focus by default on memory actually recalled or used during the task;
2. compare it with the newest code, docs, runtime results, or business facts produced by the task;
3. replace old truth rather than append correction history;
4. remove active memory that no longer has future value;
5. save new conclusions only if they pass the retention test;
6. remove consumed TODOs when appropriate;
7. update verification state only when real local or runtime verification occurred.

The server does not automatically record a task read-set and does not automatically decide semantic staleness. That is part of your reasoning work.

---

## 9. TODO Lifecycle

TODO means:

> **Worth resuming later, but not worth thinking about now.**

After resuming a TODO:

1. bring the intent back into context;
2. complete the work;
3. save any durable conclusions that actually emerge;
4. delete the TODO once it no longer needs to be resumed.

Do not keep checked TODOs as permanent history.

---

## 10. Verification and Privacy

Use `memory_mark_verified` only after a clearly defined local or runtime verification has actually been performed.

Do not mark something verified because old memory said it, because it seems plausible, or because a test probably passes.

`last_verified=unverified` (the canonical unverified literal) / `staleness=review` is not an automatic semantic-obsolescence detector.

The secret scanner is not a general privacy classifier.

If the user does not want something to become a durable Git-backed asset, do not write it in the first place.

---

## 11. Migrating a Legacy Memory Store

The migration approach is intentionally settled.

### 11.1 Learn MemAuthority Before Reading the Legacy Store

Before importing, understand:

- the retention test;
- the four roles;
- Context / Memory / TODO / Forget;
- convergence toward current truth;
- the public Vault structure for detached authoring;
- the exact Managed mutation schema.

Then read the legacy material.

### 11.2 You Are the Semantic Importer

Legacy content can be anything you can actually access and understand:

- Markdown;
- plain text;
- JSON exports;
- prompt files;
- legacy agent memory;
- project handoffs;
- mixed sources.

> **Format-agnostic at the Agent layer; deterministic at the MemAuthority layer.**

Do not wait for MemAuthority to ship a platform-specific importer. You understand the source and translate its semantics into MemAuthority.

Do not invent runtime topology during migration. Runtime metadata is an optional advanced capability, hidden by default when absent. Only migrate work/deployment environments when they are durable facts that materially help future work; otherwise leave runtime disabled.

### 11.3 Import Is Distillation

For each legacy item, decide whether to:

- keep it;
- merge it;
- update it to current truth;
- convert it into a TODO;
- keep detailed evidence only in an external canonical source;
- drop it from active memory.

Do not inherit the old system's retention decision merely because the content existed there.

Unless the user has explicitly authorized direct migration, a compact review set is recommended first:

- keep;
- merge;
- TODO;
- forget.

### 11.4 Large-Scale v1 Migration

```text
memauthority init
  -> stop or do not start managed ownership
  -> read this guide and the public Vault contract
  -> read and distill the legacy material
  -> build the canonical Vault through detached authoring
  -> memauthority validate --json
  -> repair structural and safety issues
  -> user review as needed
  -> Git commit
  -> managed serve
```

Do not edit files or Git directly while Managed runtime ownership is active.

In current v1, `memory_create_project` creates only the minimal handoff scaffold, and Managed handoff operations cannot arbitrarily insert new H2 sections.

Do not pretend a rich first import can be completed through a Managed insertion operation that does not exist.

---

## 12. Do Not Treat MemAuthority As

- a raw chat archive;
- a complete human knowledge base;
- a large-corpus RAG store;
- a deadline / owner / schedule manager;
- a duplicate copy of canonical code, docs, or issues;
- a system that automatically collects everything that might someday be useful.

What belongs here is:

> **Conclusions, state, and experience that future agents should not have to rediscover from scratch.**
