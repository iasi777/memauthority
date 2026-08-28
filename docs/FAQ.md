# V-Memory FAQ

[中文](FAQ_ZH.md)

This FAQ covers the questions most likely to recur in day-to-day use.

For exact APIs, schemas, security behavior, and compatibility rules, refer to the versioned public contract under [`contract/v1.1/`](contract/v1.1/).

---

## Does V-Memory Automatically Decide What Is Worth Remembering?

No.

V-Memory is responsible for reliable state, not for deciding what matters to the user or replacing the agent's semantic judgment.

Normally, the agent proposes candidates based on the current task, and the user decides what to keep, edit, or discard.

If the user only says “record this task,” the agent can use a conservative default: capture future-relevant `progress` instead of promoting everything into long-term rules.

---

## Why Not Keep Using `MEMORY.md`?

If `MEMORY.md + Agent` already works well enough, keep using it.

V-Memory serves a different need: memory that can converge over time while edits, concurrent writes, retries, and interruptions all have explicit boundaries.

It is not designed to force every user to migrate.

---

## Does V-Memory Save Every Conversation?

No, and it should not.

Full chats, pleasantries, temporary experiments, hidden reasoning, and failures with no future value should not enter long-term memory merely because they might “be useful someday.”

A simple test is:

> Would a future agent knowing this change behavior, restore necessary state, avoid substantial re-reasoning, or prevent a repeated mistake?

If not, it probably should not be saved.

---

## Won't “Record This Task” Make Memory Messy Over Time?

That risk cannot be eliminated by promising “automatic cleanup.”

V-Memory separates responsibilities instead:

- the agent selects content conservatively;
- V-Memory guarantees revision checks, CAS, idempotency, validation, secret scanning, and transaction safety;
- later real tasks continue updating or deleting active memory that has lost value.

So `progress` is a low-cost entry point, not a permanent archive.

---

## How Do I Choose Among the Four Roles?

Remember the intent of each role:

- `handoff`: what must be known to take over the project now;
- `rules`: what must still be followed in the future;
- `progress`: which milestones still matter later;
- `pitfalls`: which failure modes may recur.

Do not fill all four roles merely for structural completeness.

---

## Is TODO a Task Manager?

No.

A V-Memory TODO means:

> An intent worth resuming later, but not worth occupying attention now.

It does not manage deadlines, owners, schedules, dependencies, or team coordination.

A completed TODO should usually be removed. Any durable conclusion produced by the work can then be saved separately as Memory.

---

## Why Not Load All Memory at the Start of Every Task?

Because more context does not automatically produce better judgment.

The correct pattern is not a fixed recall ritual. Choose the smallest action based on what is already known:

- current context is sufficient -> do not call V-Memory;
- project is uncertain -> identify the project first;
- exact URI / section is known -> read it directly;
- project is known but location is not -> search within that project;
- overall project state is needed -> `handoff` is usually the best first resource;
- expand only what is genuinely relevant.

> **Search results are coordinates, not context.**

Search results locate useful material; they are not content that must be injected into the prompt automatically.

---

## Can Multiple Agents Overwrite Each Other?

V-Memory follows this model:

> **Multi-Agent, Single Authority.**

Multiple agents can share the same memory, but writes must be based on the current revision.

If another agent has already changed the resource, a write based on an older revision should conflict instead of silently overwriting newer content.

V-Memory exposes the conflict. The agent must reread and resolve the semantics.

---

## Does Passing Validation Mean the Memory Is Correct?

No.

Validation checks structural, safety, and deterministic constraints such as Vault format, paths, date synchronization, UTF-8, source limits, and high-confidence secrets.

It does not decide:

- whether a memory is actually important;
- whether a conclusion is intelligent;
- whether content is semantically obsolete;
- whether the user wants it retained long-term.

Active memory may still require human or agent review after validation passes.

---

## I Have Thousands of Lines in `MEMORY.md`. Do I Have to Reformat Them Manually?

No.

Let the agent perform semantic migration:

1. learn V-Memory retention and role semantics;
2. read the legacy store;
3. deduplicate, merge, update, and retire content;
4. write only what remains worth keeping long-term into the new Vault;
5. validate, review, commit, then move into Managed operation.

Import is distillation, not copying.

---

## Why Not Build a Universal Importer?

Because the hard part is not whether the source is Markdown, JSON, or a platform export. The hard part is deciding what is still true, what conflicts, what is merely history, and what should never be retained long-term.

Those are semantic judgments, best handled by an agent that understands both the source and the user's intent.

---

## Can I Edit Vault Files Directly While the Managed Service Is Running?

Do not do that concurrently.

Detached authoring permits direct Git / Markdown / YAML editing. Once Managed ownership is active, Authority mutations should use the controlled MCP path.

For large direct restructuring, stop Managed ownership, complete detached authoring, validate, review, commit, and then restart the Managed service.

---

## If I Delete a Memory, Is It Also Gone from Git History?

No.

Active V-Memory stores what future agents still need to know now. Git stores the history of Authority changes.

Deleting active memory does not physically erase historical commits.

If something should never become a durable Git-backed asset, the safest approach is not to write it in the first place. Actual historical deletion requires a separate data-erasure / Git-history-rewrite process.

---

## Can the Secret Scanner Detect All Private Information?

No.

V-Memory blocks high-confidence secret patterns, but it is not a general privacy classifier.

Whether a home address, personal experience, business secret, or unpublished plan should be retained is still a user-and-agent decision.

---

## Is V-Memory a RAG System, Knowledge Base, or Chat Archive?

No.

- large-document retrieval belongs in RAG / search systems;
- complete project knowledge belongs in README, docs, Wiki, or ADRs;
- raw conversations belong in chat or logging systems;
- deadlines / owners / schedules belong in task-management tools.

V-Memory keeps:

> **What future agents should not have to rediscover from scratch.**
