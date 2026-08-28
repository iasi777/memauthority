# V-Memory Onboarding

[中文](ONBOARDING_ZH.md)

> Purpose: first-time setup and migration guide for V-Memory users.
> Exact APIs, schemas, and security behavior are defined by the versioned public contract under [`contract/v1.1/`](contract/v1.1/).

This guide answers three questions:

1. How do I build a Vault from scratch?
2. How do I migrate an existing `MEMORY.md` or legacy memory store?
3. Once the Vault exists, how do I connect an agent and start normal use?

---

## 1. Understand One Division of Responsibility

- **You decide what deserves long-term retention.**
- **The agent interprets, organizes, migrates, and maintains it.**
- **V-Memory provides reliable reads and reliable writes.**

> **You decide what to remember; let the agent handle the maintenance.**

V-Memory is not an automatic chat collector, and you do not need to learn the entire Markdown schema by hand before using it.

---

## 2. Choose Your Starting Path

### Path A: Start from Scratch

Use this when you do not yet have long-term agent memory worth migrating.

### Path B: Import an Existing Memory Store

Use this when you already have material such as:

- `MEMORY.md`;
- prompt files;
- project handoffs;
- Markdown / text notes;
- JSON exports;
- legacy agent memory;
- chat summaries;
- multiple conflicting historical memory sources.

For many V-Memory users, migration is the main reason to adopt it.

---

## 3. Starting from Scratch

### 3.1 Create an Empty Vault

```sh
v-memory init ./vault
```

This creates an empty Git-backed Vault and its initial commit.

It does not create sample projects or guess what should be remembered.

### 3.2 Let the Agent Build the Minimum Useful Memory

For the first full setup, let an agent with local file and Git access perform detached authoring before Managed `serve` is started.

Give the agent:

- [`AGENT-GUIDE.md`](AGENT-GUIDE.md);
- the current project's code, docs, and necessary context;
- any durable information you explicitly want retained.

The agent should create only what already has a reason to exist.

Do not prefill every role simply to make the Vault look complete.

`handoff` is required; the other roles can appear naturally through real work.

### 3.3 Validate

```sh
v-memory validate --json ./vault
```

Validation checks:

- Vault structure;
- role/frontmatter rules;
- path and symlink safety;
- date and cross-file synchronization;
- UTF-8 / source limits;
- high-confidence secrets.

Validation does **not** decide whether content is intelligent, important, or semantically current.

Even after structural validation passes, review active memory when appropriate.

### 3.4 Commit and Start the Managed Service

After reviewing the result, commit it to Git and start:

```sh
v-memory serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

Remember:

- omit `--write-enabled` for read-only mode;
- `state-dir` must be outside Vault Authority;
- Managed admission requires a valid, committed, clean Vault;
- direct detached editing and Managed runtime ownership must not run concurrently.

---

## 4. Importing an Existing Memory Store

The migration approach is deliberate:

> **The agent performs semantic migration; V-Memory does not need a custom importer for every legacy format.**

### 4.1 Why the Agent Handles Import

The hard part of legacy migration is not file format. It is semantics:

- what is still true;
- what conflicts;
- what is merely historical;
- what is private;
- what belongs to current state, durable rules, milestone progress, pitfalls, or TODO;
- what should leave active memory entirely.

Those decisions belong with an agent that understands both the task and the user's intent.

> **If the agent can access and understand the legacy content, it can migrate it.**

### 4.2 Correct Order of Operations

The agent should understand V-Memory before reading the legacy store:

1. read [`AGENT-GUIDE.md`](AGENT-GUIDE.md);
2. understand Context / Memory / TODO / Forget;
3. understand the four roles;
4. understand that active memory should converge toward current truth rather than preserve full correction history;
5. only then read the legacy material;
6. deduplicate, merge, update, classify, and retire content;
7. produce a valid V-Memory structure.

> **Format-agnostic at the Agent layer; deterministic at the V-Memory layer.**

### 4.3 Import Is Distillation, Not Copying

Do not keep something merely because the old system once stored it.

For every legacy item, choose whether to:

- keep it as current Memory;
- merge it with other content;
- update it to a newer truth;
- convert it into a TODO;
- leave the full detail in its original canonical document and store only a concise conclusion or pointer;
- remove it from active memory.

Reducing a 5,000-line legacy store to 500 high-signal lines can be a better migration than preserving everything.

### 4.4 Recommended User Instruction

```text
Read V-Memory's memory rules first, then inspect this legacy memory store.
Deduplicate, merge, update, and reorganize everything still worth keeping long-term.
Do not import material that is obsolete, repetitive, purely historical, temporary,
or unsuitable for long-term retention.
Show me the migration candidates first, then write only after I approve them.
```

If the source and agent are already trusted, the user can explicitly authorize direct migration instead.

Candidate review is a recommended workflow, not a server-enforced step.

### 4.5 Common Legacy-to-V-Memory Mapping

- current takeover state -> `handoff`
- durable decisions and constraints -> `rules`
- milestone progress that still explains the current state -> `progress`
- recurring failure modes -> `pitfalls`
- unfinished intent worth resuming later -> handoff TODO
- complete design docs and reference material -> keep in their canonical location, with only concise conclusions or pointers in V-Memory when useful
- obsolete, duplicate, temporary, private, or purely historical material -> do not place in active memory

This is guidance, not automatic server classification.

### 4.6 Large-Scale v1 Migration

```text
v-memory init
  -> stop or do not start managed ownership
  -> Agent reads V-Memory guidance
  -> Agent reads and distills the legacy store
  -> detached authoring builds the canonical Vault
  -> v-memory validate --json
  -> user reviews as needed
  -> Git commit
  -> managed serve
```

In current v1, `memory_create_project` creates only a minimal handoff scaffold, and Managed handoff mutations cannot insert arbitrary new H2 sections.

For that reason, detached authoring is the intended v1 path for a rich first handoff or a large legacy migration, not a temporary workaround.

### 4.7 Incremental Migration Later

After the Managed service starts, remaining small legacy items can be absorbed gradually through normal typed mutations.

For a large restructuring of the entire Vault:

1. stop Managed ownership explicitly;
2. perform detached authoring;
3. validate;
4. review;
5. commit;
6. restart the Managed service.

Do not modify files or Git directly while Managed ownership is active.

---

## 5. Connect V-Memory to an Agent

Normal Managed use is exposed through MCP.

For local use, stdio is the simplest option and the MCP client can launch the process directly:

```sh
v-memory serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

Different agents and clients use different configuration formats, but the essential information is the same:

- command: `v-memory`
- arguments: `serve` plus the required options

See [`MCP-CONFIG.md`](MCP-CONFIG.md) for the stable connection model.

Once connected, the agent automatically receives:

- tool descriptions;
- input schemas;
- annotations;
- resource metadata.

These describe the fields, mechanical constraints, and current tool behavior exposed by the running service.

Normal Managed use does not require you to restate parameter rules every time, and the agent should not hard-code an old release's fields when the current tool schema is available.

[`AGENT-GUIDE.md`](AGENT-GUIDE.md) adds the cross-tool principles that are most useful during:

- detached bootstrap;
- legacy migration;
- ambiguous role selection;
- low-effort task logging and later maintenance.

### HTTP

Before remote HTTP exposure, read:

- [`../SECURITY.md`](../SECURITY.md)
- [`contract/v1.1/transport-auth.md`](contract/v1.1/transport-auth.md)

Do not turn a local stdio example into a public listener by simply changing the transport.

---

## 6. First Day-to-Day Use

### 6.1 Recall

Use this only when the current session genuinely lacks project context:

```text
Check this project's V-Memory and read only what you actually need.
```

The agent should:

1. route the project only when necessary;
2. use handoff first when overall takeover state is needed;
3. search further only when the task requires it;
4. treat search results as coordinates rather than automatic context;
5. read only the sections that are actually relevant.

### 6.2 Curated Recording

```text
List what from this task is worth remembering long-term. I will decide what gets written.
```

### 6.3 Low-Effort Recording

```text
Record this task.
```

The default should be conservative `progress`, not chat duplication or automatic promotion into long-term rules.

### 6.4 Maintain Existing Memory

```text
Review and improve the V-Memory entries we actually used during this task, and clean up anything obsolete.
```

By default, maintain only memory that was genuinely used during the task.

### 6.5 TODO

```text
This direction is worth revisiting later. Put it in V-Memory TODO so it does not occupy the current context.
```

After completion, the TODO should usually be removed. Any durable conclusion can be stored separately as Memory.

---

## 7. First-Use Checklist

- [ ] Vault was created with `v-memory init`;
- [ ] first full setup or legacy migration happened during detached authoring;
- [ ] `v-memory validate --json` passes;
- [ ] active memory was reviewed as needed, not merely assumed correct because validation passed;
- [ ] Vault is committed and the worktree is clean;
- [ ] `state-dir` is outside the Vault;
- [ ] Managed write access is enabled or disabled intentionally;
- [ ] MCP client is configured correctly;
- [ ] security requirements are complete before HTTP exposure;
- [ ] the agent understands on-demand recall, conservative recording, and convergence toward current truth.

---

## 8. Avoid These Patterns

- Do not bulk-copy complete chat history into V-Memory;
- do not make the user manually translate a legacy store into four roles;
- do not treat validation as proof of semantic quality;
- do not edit the Vault directly while Managed ownership is active;
- do not load every role at the start of every task;
- do not keep checked TODOs as permanent history;
- do not imply that deleting active memory erases Git history;
- do not redesign migration as a server-side generic importer.
