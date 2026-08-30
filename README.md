# MemAuthority — Git-Backed Long-Term Memory for AI Agents

[中文](README_ZH.md)

English is the primary documentation language. User-facing guides provide Chinese counterparts with the `_ZH.md` suffix. Frozen normative snapshots, changelogs, legal notices, provenance, contribution policy, and deployment records remain English-authoritative unless explicitly paired.

**Effortless control over agent memory — leave the maintenance to the agent.**

MemAuthority is a Git-backed, conflict-safe long-term memory system for AI agents, exposed through MCP (Model Context Protocol). **Formerly V-Memory**; the project was renamed to give it a unique, searchable identity.

Keywords: **AI agent memory**, **long-term memory**, **MCP memory server**, **Git-backed memory**, **agent memory infrastructure**.

Rather than attempting to record every single detail, it focuses on distilling:

> **Lessons and conclusions that future agents should never have to rediscover from scratch.**

A clear division of responsibility:
- **You** act as the gatekeeper: deciding what is truly worth keeping for the long haul;
- **The Agent** handles execution: comprehending, summarizing, retrieving, updating, and pruning memories;
- **MemAuthority** provides the safety net: guaranteeing reliable storage and on-demand retrieval, while delivering deterministic system protection against version conflicts, retry duplicates, and unexpected interruptions.

---

## Who Is This For?

MemAuthority is designed for users who prioritize the **quality** of long-term memory.

You may have experienced the lack of control with built-in platform memories:
- Unclear *why* something was remembered;
- Uncertain *when* it will be recalled;
- Unable to verify whether information has become obsolete;
- Left with no clean way to make precise edits or deletions.

Or perhaps you have tried maintaining a `MEMORY.md` file, only to watch it grow bloated and messy over time — eventually confusing the agent rather than helping it.

MemAuthority shines in scenarios where you:

> **Want to invest minimal effort curating memory quality, while delegating all routine organization to the agent.**

"Investing effort" does not mean manually editing files all day. It means making high-leverage decisions at key moments:
- Is this information worth keeping long-term?
- Has an earlier conclusion become obsolete or invalid?
- Does any of this involve sensitive data or secrets?
- Does the agent's proposed curation align with your actual intent?

All mechanical heavy lifting — formatting, categorizing, archiving, and targeted retrieval — is handled entirely by the agent.

---

## Why Not Just Use `MEMORY.md`?

If your project's memory is small, rarely changes, or you simply do not want to spend any attention on memory maintenance, sticking with a plain `MEMORY.md` is the easiest choice.

MemAuthority addresses a higher-order need:

> **Turning long-term memory maintenance into a reliable, controllable, and engineered workflow.**

It provides far more than just "letting an agent edit Markdown." It delivers a robust operational architecture:
- **Precise On-Demand Loading**: Loads memory only when a task actually needs it, preventing irrelevant noise from bloating or polluting the context window;
- **Clear Role Separation**: Distinctly separates handoff state, standing rules, milestone progress, and pitfall avoidance;
- **Concurrency & Version Safety**: Enforces strict version checks (CAS — Compare-And-Swap) before writes, preventing multiple agents from silently overwriting fresh content with stale revisions;
- **Idempotency & Deduplication**: Safe against retries caused by network glitches or aborted runs without creating duplicate memories;
- **Convergence & Full History**: Keeps the active working memory lean and compact, while relying on Git for complete, auditable revision history;
- **Transactions & Crash Recovery**: Features explicit journaling and recovery mechanisms in case of interrupted or failed writes.

In short:

> **`MEMORY.md` optimizes for "giving anyone a zero-barrier memory file"; MemAuthority optimizes for "enabling quality-conscious users to sustainably maintain high-grade memory over the long haul."**

---

## Everyday Workflow

In an ideal workflow, MemAuthority stays quietly behind the scenes. You interact with the agent using natural, concise instructions:

### 1. Retrieve on Demand at Task Start
*Use only when the current session genuinely lacks project context:*
> **"Check the MemAuthority for this project first — pull in only what you need."**

The agent reads the current handoff state (`handoff`) first, then retrieves specific sections only as the task requires. Never dump the entire vault into the context by default.

### 2. Curate What to Remember
> **"List the takeaways from this task worth keeping long-term — I'll decide what to save."**

The agent distills candidate items, and you make the final call on what to keep, edit, or discard.

### 3. Quick, Low-Friction Logging
> **"Record this task."**

A common, low-effort command. By default, the agent adopts a conservative strategy — logging it as a low-risk `progress` entry without altering long-term `rules` on its own.

### 4. Post-Task Maintenance
> **"Clean up and optimize the memories we referenced during this task, and prune anything obsolete."**

The agent targets only the memories loaded and referenced during the current task, avoiding unnecessary full-vault scans.

### 5. Park Ideas for Later to Free Up Context
> **"This direction is worth exploring later — drop it in MemAuthority TODO so it doesn't clutter our current context."**

Here, a TODO is not a project management issue tracker, but rather:
> **A staging ground for ideas worth revisiting later, without consuming cognitive bandwidth today.**

Once acted upon, the TODO item should be deleted, and any durable conclusions that emerge are promoted to long-term memory.

---

## "Record this task" — How to Avoid Memory Bloat?

MemAuthority does not require you to deliberate over "should this go into rules or progress" every time you want to record something.

When you give an open-ended command like *"Record this task"*, the agent's default behavior is:
- **Default to `progress`**;
- Record only the core deliverables completed in this session;
- Record only meaningful state transitions;
- Record only key context needed for future continuation.

**What should be actively filtered out by default:**
- Full chat transcripts, conversational filler, and pleasantries;
- Granular, operation-by-operation action logs and trial-and-error details;
- Ephemeral failures with no future reuse value;
- Duplicate facts already documented in the vault;
- Transient, outdated, sensitive, or purely speculative content.

Furthermore, an agent must never elevate something to a long-term `rules` entry simply because it "sounds important." Durable rules, handoff states, and pitfall lessons should be distilled progressively through real-world work.

**System-level guardrails provided by MemAuthority:**
- Enforces a standard schema for all written content;
- Restricts mutations based on memory roles (Role-based access);
- Automatically scans and blocks high-confidence secrets and credentials;
- Uses idempotency mechanisms to prevent retries from piling up duplicate entries;
- Uses version validation (CAS) so stale revisions cannot silently overwrite newer revisions;
- Validates candidate vault integrity before committing writes;
- Supports journaled transactions with crash recovery.

The system's promise is not that "a low-quality note will never be written," but rather:

> **The agent records conservatively, MemAuthority guarantees underlying state integrity, and ongoing real-world work continuously refines and prunes the memory.**

Consistency operates on two distinct layers:
- **Semantic Consistency**: Maintained by the agent, reconciling new task outcomes with existing knowledge;
- **State Consistency**: Enforced by MemAuthority, backed by Git Authority, revision CAS, idempotency, and transactional journaling.

Remember: `progress` is a low-friction entry point for milestone logging, not an immutable, append-only archive.

---

## What If I Already Have `MEMORY.md` or Legacy Notes?

No need to rewrite everything by hand.

For legacy migration, MemAuthority takes a principled approach:

> **Let the agent perform semantic migration, rather than building custom importers for every legacy format into MemAuthority.**

As long as the agent can read and understand your legacy material, it can migrate from any source:
- Existing `MEMORY.md` files;
- General Markdown or plain text documents;
- JSON exports;
- System prompt files;
- Historical handoffs and chat summaries;
- Conflicting multi-source notes;
- Any other text format an LLM can parse.

**Standard Migration Flow:**
1. The agent learns MemAuthority's structural specifications and memory standards;
2. The agent reads the legacy notes;
3. It deduplicates, merges, updates, and categorizes information;
4. It strips out obsolete facts, redundant entries, raw logs, ephemeral notes, and unsuitable content;
5. It exports the curated output into the standard MemAuthority vault structure.

You can simply instruct your agent:

> **"Read MemAuthority's memory specification first, then inspect this legacy memory file. Deduplicate, merge, update, and restructure anything worth keeping long-term. Remove anything outdated, repetitive, raw logs, ephemeral notes, or unfit for long-term storage. Propose a migration draft for my review before writing."**

Migration is fundamentally **a curation decision**, not a mechanical copy-paste.

Distilling a bloated 5,000-line legacy dump down to 500 lines of high-signal memory is often the hallmark of a successful migration.

For initial onboarding or large-scale migration, the recommended v1 workflow is:

```text
init (initialize empty vault)
  -> Agent curates vault in detached mode
  -> validate (verify integrity & schema)
  -> Human review
  -> Git commit (commit revision)
  -> managed serve (launch MCP service)
```

For the complete guide, see [`docs/ONBOARDING.md`](docs/ONBOARDING.md).

---

## What Goes into MemAuthority?

Long-term memories in MemAuthority are strictly divided into four roles:

### `handoff` (Handoff State)
The **minimal essential context** required for an agent to immediately take over the project.
Keep it concise, actionable, and continuously updated. It should never become a bloated second README.

### `rules` (Long-Term Rules)
Architectural constraints, standing decisions, and behavioral guidelines future agents must follow.
Record only settled decisions, not protracted debates or historical discussions.

### `progress` (Milestone Progress)
Key milestones and state transitions that remain relevant for future work.
This is the lowest-friction entry point for logging, but not an append-only transaction log.

### `pitfalls` (Pitfalls & Lessons)
Recurring failure modes, non-obvious traps, and proven workarounds that future agents might encounter.
Routine errors and one-off typos do not belong here.

A reliable heuristic:

> **Will knowing this change a future agent's decisions, save substantial trial-and-error, or prevent repeated mistakes?**

If not, it probably does not belong in long-term memory.

---

## Why Not Dump All Memories into Context?

Because **remembering more does not mean reasoning better.**

MemAuthority uses a **Progressive Recall** pattern:
1. If the current conversation already has sufficient context, skip calling MemAuthority entirely;
2. If the target project is ambiguous, locate and confirm the project first;
3. If an exact URI or section is known, perform a direct, targeted read;
4. If the project is known but the exact section is not, search within that project's scope;
5. If quick global orientation is needed, `handoff` is typically the first resource to check;
6. The agent evaluates search results and loads *only* the specific sections that are genuinely relevant.

> **Search results are coordinates, not context.**

This progressive approach not only slashes token overhead, but more crucially shields the model from hallucination and confusion caused by stale or superficially similar snippets.

---

## Won't Multiple Agents Cause Conflicts?

MemAuthority v1 is built around a core principle:

> **Multi-Agent, Single Authority.**

Multiple agents can read from and write to the same memory vault, but the Authority maintains a single, linear, deterministic version history.

If Agent A updates the memory while Agent B attempts a mutation based on a stale revision, MemAuthority rejects the write with an explicit conflict error, preventing silent overwrites.

Agent B must then re-fetch the latest state and decide whether to merge, overwrite, abort, or prompt the user for clarification.

MemAuthority does not arbitrate subjective opinions; its responsibility is clear:

> **Surface concurrency conflicts explicitly, ensuring divergent edits never silently corrupt the vault.**

---

## Active Memory Is Not an Archive

Git faithfully records the complete evolution of the Authority for full auditability. Active Memory contains **only what remains valuable for future agents today**.

- When rules change, update the rules in place;
- When state evolves, update the handoff state;
- When content becomes obsolete, prune it from active memory.

> **History preserves evolution; memory maintains convergence.**

Pruning an item from active memory does not erase it from Git history. However, truly sensitive secrets or ephemeral noise should be filtered out before they ever enter the vault.

---

## Installation

MemAuthority requires Git and Go 1.26.5 or later. Install the current stable release with:

```sh
go install github.com/iasi777/v-memory/cmd/memauthority@v1.3.0
```

The v1.x Go module identity intentionally remains `github.com/iasi777/v-memory` for compatibility even though the canonical repository and product name are now MemAuthority. GitHub redirects the former repository URL to `iasi777/memauthority`.

Verify:

```sh
memauthority version
```

Expected output:

```text
memauthority 1.3.0
```

Existing automation may continue installing and invoking the compatibility executable:

```sh
go install github.com/iasi777/v-memory/cmd/v-memory@v1.3.0
v-memory version
```

No prebuilt release binaries are currently published. To build from a source checkout instead:

```sh
go build -trimpath -o ./memauthority ./cmd/memauthority
./memauthority version
```

Release CI runs the test suite, vet, and native CLI build on Linux, macOS, and Windows; Linux CI also verifies the production Linux/ARM64 target.

---

## Getting Started

### 1. Initialize an Empty Vault
```sh
memauthority init ./vault
```

### 2. Onboarding & Curation
When bootstrapping a new vault or migrating a large legacy corpus, have an agent with file and Git permissions read:
[`docs/AGENT-GUIDE.md`](docs/AGENT-GUIDE.md)

Once curated, validate the vault:
```sh
memauthority validate ./vault
```

### 3. Commit & Launch Service
Review changes, commit them to Git, and launch the Managed MCP Service:
```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

- Omitting `--write-enabled` runs the service in **read-only mode**;
- `state-dir` must be located outside the Vault Authority directory;
- Do not mix Detached mode (direct file edits) and Managed mode (running service) concurrently;
- Optional declarative runtime metadata is **off by default** when a Vault has no registered runtime. Most single-machine users should leave it off. Existing Vaults with `runtime_resource` enable the runtime tools automatically; use `--runtime-enabled` only when you intentionally want to start recording work/deployment topology.

For local setups, the simplest approach is having your MCP client spawn the command directly over stdio.

Once connected, MemAuthority automatically provides tool definitions, input schemas, annotations, and resource metadata to the agent. During day-to-day managed operation, you do not need to re-explain parameter schemas to your agent.

See [`AGENT-GUIDE.md`](docs/AGENT-GUIDE.md) for cross-cutting usage guidelines (on-demand recall, conservative recording, role selection, and legacy migrations).

If exposing via HTTP transport, be sure to review [`SECURITY.md`](SECURITY.md) and the frozen v1 Transport / Auth specifications first.

---

## What MemAuthority Is Not

If your primary need falls into one of these categories, dedicated alternatives are a better fit:
- **Fully automatic personal preference tracking**: Better served by built-in model/platform memory;
- **Tiny, static, manual notes**: A simple `MEMORY.md` is fine;
- **Full-text search across large technical docs**: Standard RAG / documentation search engines;
- **Human-facing project documentation**: Dedicated READMEs, wikis, ADRs, or docs folders;
- **Tracking schedules, assignees, deadlines, and dependencies**: Project management tools like Jira or Linear.

MemAuthority is not a chat archiver, not an exhaustive knowledge base, and not a task tracker.

Its single focus is preserving:

> **Lessons and conclusions that future agents should never have to rediscover from scratch.**

---

## Building and Verification

```sh
go test ./...
go vet ./...
go mod verify
go build -trimpath -o ./memauthority ./cmd/memauthority
```

---

## Public Contract

The current public compatibility baseline is **v1.3.0**, the first release under the MemAuthority brand. The frozen v1.1/v1.2 V-Memory snapshots remain unchanged historical contracts.

Authoritative definitions of Vault storage formats, MCP tools, Managed runtime behavior, mutation/refusal rules, security boundaries, transport/authentication behavior, and compatibility policy are maintained under [`docs/contract/v1.3/`](docs/contract/v1.3/).

*This README and the user guides are explanatory; the versioned contract is authoritative when details differ.*

## Version

```sh
memauthority version
memauthority --version
```

For v1.3.0, the primary command prints `memauthority 1.3.0`; the compatibility command prints `v-memory 1.3.0`.

## Security

Before exposing the service over HTTP transport, make sure to read [`SECURITY.md`](SECURITY.md).

- Write access is disabled by default; enable it explicitly with `--write-enabled`;
- HTTP mutations must strictly adhere to the OAuth and security requirements in the frozen v1 spec;
- MemAuthority includes built-in high-confidence secret scanning, but this does not replace full-fledged secret detection tooling.

## Related Documentation

- [`docs/ONBOARDING.md`](docs/ONBOARDING.md) — Quickstart, legacy migration, and first-time setup guide
- [`docs/MCP-CONFIG.md`](docs/MCP-CONFIG.md) — stdio / HTTP connection guide and configuration specs
- [`docs/AGENT-GUIDE.md`](docs/AGENT-GUIDE.md) — Practical rules for agents on recall, recording, maintenance, and migration
- [`docs/FAQ.md`](docs/FAQ.md) — Frequently asked questions, design boundaries, and trade-offs
- [`examples/README.md`](examples/README.md) — Runnable sample Vault and first local MCP session
- [`SECURITY.md`](SECURITY.md) — Supported security line, deployment boundaries, and private reporting
- [`docs/contract/v1.3/`](docs/contract/v1.3/) — Versioned v1.3 public contract

## License

Released under the Apache License 2.0. See [`LICENSE`](LICENSE) for details.

Attribution and third-party notices are documented in [`NOTICE`](NOTICE) and [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
