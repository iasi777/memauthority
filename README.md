# MemAuthority — Git-Backed Long-Term Memory for AI Agents

[中文](README_ZH.md)

MemAuthority is a Git-backed long-term memory system for AI agents, exposed through MCP (Model Context Protocol). It lets a new session, another machine, or another agent recover the project's accumulated context and continue working instead of rediscovering the same decisions and pitfalls.

Rather than attempting to record every single detail, it focuses on distilling:

> **Lessons and conclusions that future agents should never have to rediscover from scratch.**

A clear division of responsibility:
- **You** act as the gatekeeper: deciding what is truly worth keeping for the long haul;
- **The Agent** handles execution: understanding, retrieving, summarizing, updating, and pruning memories;
- **MemAuthority** provides the safety net: keeping long-term memory reliable and available on demand while protecting it from stale writes, duplicate retries, and interrupted updates.

The agent still decides what the content means and whether it should change. MemAuthority makes sure the memory that remains is reliable over time.

---

## Who Is This For?

MemAuthority is designed for users who prioritize the **quality** of long-term memory.

You may have experienced the lack of control with various memory tools:
- Unclear *why* something was remembered;
- Uncertain *when* it will be recalled;
- Unable to verify whether information has become obsolete;
- Left with no clean way to make precise edits or deletions.

Or perhaps you have tried maintaining a memory store, only to watch it grow bloated and messy over time — eventually confusing the agent rather than helping it.

MemAuthority is a good fit when:

> **Want to invest minimal effort curating memory quality, while delegating all routine organization to the agent.**

"Investing effort" does not mean manually editing files all day. It means making high-leverage decisions at key moments:
- Is this information worth keeping long-term?
- When should the memory be reviewed or cleaned up?
- Does any of this involve sensitive data or secrets?
- Does the agent's proposed curation align with your actual intent?

All mechanical heavy lifting — formatting, categorizing, archiving, and targeted retrieval — is handled entirely by the agent.

---

## Why Not Just Use `MEMORY.md`?

If your project's memory is small, rarely changes, or you simply do not want to spend any attention on memory maintenance, sticking with a plain `MEMORY.md` is the easiest choice.

Once that file stops being just a note and becomes long-term project state that future sessions need to trust, MemAuthority starts to become useful.

It addresses a higher-order need:

> **Turning long-term memory maintenance into a reliable, controllable, and engineered workflow.**

It provides far more than just "letting an agent edit Markdown." It delivers a robust operational architecture:
- **Precise On-Demand Loading**: Loads memory only when a task actually needs it, preventing irrelevant noise from bloating or polluting the context window;
- **Clear Role Separation**: Distinctly separates handoff state, standing rules, milestone progress, and pitfall avoidance;
- **Concurrency & Version Safety**: Enforces strict version checks (CAS — Compare-And-Swap) before writes, preventing stale revisions from silently overwriting fresh content;
- **Idempotency & Deduplication**: Safe against retries caused by network glitches or aborted runs without creating duplicate memories;
- **Convergence & Full History**: Keeps the active working memory lean and compact, while relying on Git for complete, auditable revision history;
- **Transactions & Crash Recovery**: Features explicit journaling and recovery mechanisms in case of interrupted or failed writes.

A useful way to think about it is:

> **Authority stores the state a future agent should inherit now; Git stores the full history.**

In short:

> **`MEMORY.md` optimizes for simplicity; MemAuthority optimizes for keeping long-term memory reliable as the project grows.**

---

## Everyday Workflow

You interact with the agent using natural, concise instructions:

### 1. Retrieve on Demand at Task Start
*The agent decides whether to invoke memory based on actual task conditions, similar to `MEMORY.md`, or you can instruct the agent to read specific memories:*
> **"Check this project's MemAuthority — pull in only what you need."**

The agent chooses the shortest and most suitable retrieval path, as MemAuthority does not mandate a fixed retrieval method: read directly when the location is known, search when it is not, and use `handoff` when it needs a quick overall project handoff. If the current conversation already contains enough context, there is no need to call MemAuthority at all.

### 2. Curate What to Remember
> **"List the takeaways from this task worth keeping long-term — I'll decide what to save."**

The agent distills candidate items, and you make the final call on what to keep, edit, or discard.

### 3. Quick, Low-Friction Logging
> **"Record this task."**

A common, low-effort command. By default, the agent adopts a conservative strategy — logging it as a low-risk `progress` entry without altering long-term `rules` on its own.

### 4. Post-Task Maintenance
> **"Review the MemAuthority memory actually used in this task. Update it from what just happened or was verified, and remove anything obsolete."**

The agent that just completed the task has the freshest code, tool results, runtime facts, and user decisions. It only needs to maintain the memory it actually read and used, rather than scanning the entire Vault every time.

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

Meanwhile, an agent will not elevate something to a long-term `rules` entry simply because it "sounds important." Truly solid rules, handoff states, and pitfall lessons should be distilled progressively through real-world work.

**System-level guardrails provided by MemAuthority:**
- Enforces a standard schema for all written content;
- Restricts mutations based on memory roles (Role-based access);
- Automatically scans and blocks high-confidence secrets and credentials;
- Uses idempotency mechanisms to prevent retries from piling up duplicate entries;
- Uses version validation (CAS) so stale revisions cannot silently overwrite newer revisions;
- Validates candidate vault integrity before committing writes;
- Supports journaled transactions with crash recovery.

The goal is not to "never produce a single low-quality note," but rather:

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

## How to Prevent Unnecessary MemAuthority Memories from Entering Conversation Context?

MemAuthority supports Progressive Recall, but does not require the agent to follow a fixed ritual:
1. If the current conversation already has enough context, do not call MemAuthority at all;
2. If the project is unclear, locate and confirm it first;
3. If the exact URI or section is known, read it directly;
4. If the project is known but the location is not, search within that project;
5. If a quick overall handoff is needed, `handoff` is usually the best starting point;
6. The agent decides what is relevant and reads only what genuinely helps the current task.

> **Search results are coordinates, not context.**

The goal is simple: keep irrelevant memory out of the current conversation context, leaving the judgment of how much evidence is sufficient to the agent performing the task.

---

## Won't Multiple Agents Cause Conflicts?

MemAuthority v1 is built around a core principle:

> **Multi-Agent, Single Authority.**

Different agents or clients can take turns reading and requesting changes against the same Managed Authority, while MemAuthority maintains a single, linear, deterministic version history. v1 is still designed for a **single user and a single writer**, not as a collaborative database for simultaneous team editing.

If Agent A updates the memory while Agent B attempts a mutation based on a stale revision, MemAuthority returns an explicit conflict and rejects the write instead of letting stale state overwrite fresh state.

Agent B must then re-fetch the latest state and decide whether to merge, overwrite, abort, or ask the user.

MemAuthority does not decide which subjective opinion is correct. Its responsibility is narrower:

> **Surface concurrency conflicts explicitly, so divergent edits never silently become incorrect Authority.**

---

## How to Avoid MemAuthority Growing Bloated

Git faithfully records the complete evolution of the Authority so memory can be traced at any time, but agents do not read Git history in practical work; agents only read active memory, which can be revised freely while Git guarantees recoverability:

- When rules change, update the rules in place;
- When state evolves, update the handoff state;
- When content becomes obsolete and no longer needs recall, delete it directly.

---

## Installation

MemAuthority requires Git and Go 1.26.5 or later. Install the current stable release with:

```sh
go install github.com/iasi777/v-memory/cmd/memauthority@v1.3.2
```

The v1.x Go module identity intentionally remains `github.com/iasi777/v-memory` for compatibility even though the canonical repository and product name are now MemAuthority. GitHub redirects the former repository URL to `iasi777/memauthority`.

Verify:

```sh
memauthority version
```

Expected output:

```text
memauthority 1.3.2
```

Existing automation may continue installing and invoking the compatibility executable:

```sh
go install github.com/iasi777/v-memory/cmd/v-memory@v1.3.2
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

If exposing via HTTP transport, be sure to review [`SECURITY.md`](SECURITY.md) and the frozen v1.3.2 Transport / Auth specification first.

## Building and Verification

```sh
go test ./...
go vet ./...
go mod verify
go build -trimpath -o ./memauthority ./cmd/memauthority
```

---

## Benchmark

MemAuthority maintains a public, auditable benchmark suite that validates cross-session memory reliability under real agent workloads.

### Latest Campaign: Mutation Usability Regression (2026-09-08)

**handoff-pending-01** — Cross-session project handoff with pending work tracking

| Metric | Result |
|---|---:|
| Total runs | 30/30 PASS |
| Total sessions | 90/90 PASS |
| **Mutation operations** | **426 total** |
| **Mutation errors** | **2 (0.5%)** |
| Human semantic review | 30/30 confirmed |
| Independent audit | Claude Opus 5, HIGH confidence |

**Tested Agent Stacks** (10 rounds each):
- **Codex CLI + GPT-5.6 Sol**: 10/10 runs, **0/136 mutation errors** (0.0%)
- **Codex CLI + GPT-5.6 Luna**: 10/10 runs, **0/138 mutation errors** (0.0%)
- **Pi Agent + Gemini 3.8 Flash**: 10/10 runs, **2/152 mutation errors** (1.3%, both recovered)

**Key finding**: All tested Agent Stacks successfully preserved and recovered project state across fully independent sessions. The corrected mutation error rate of **0.5% (2/426)** demonstrates high API usability.

**Audit note**: Original report claimed 18/426 errors. Comprehensive MCP log analysis corrected this to 2/426. Both errors occurred in the same run, same tool, same validation issue. Agent recovered successfully. See audit documentation for methodology.

📊 **Complete results**: [`benchmark/results/mutation-usability-regression/`](benchmark/results/mutation-usability-regression/)  
📋 **Benchmark specifications**: [`benchmark/`](benchmark/) | [Full documentation](docs/BENCHMARK.md)  
📦 **Evidence download**: [GitHub Releases](https://github.com/iasi777/memauthority/releases) (30 complete runs, all artifacts)

---

## Public Contract

The current public compatibility baseline is **v1.3.2**. Earlier released contract snapshots remain unchanged.

Authoritative definitions of Vault storage formats, MCP tools, Managed runtime behavior, mutation/refusal rules, security boundaries, transport/authentication behavior, and compatibility policy are maintained under [`docs/contract/v1.3.2/`](docs/contract/v1.3.2/).

*This README and the user guides are explanatory; the versioned contract is authoritative when details differ.*

## Version

```sh
memauthority version
memauthority --version
```

**AI Agent Memory**, **Long-Term Memory**, **MCP Memory Server**, **Git-backed Memory**, **Agent Memory Infrastructure**, **Agent Continuity**

For v1.3.2, the primary command prints `memauthority 1.3.2`; the compatibility command prints `v-memory 1.3.2`.

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
- [`docs/contract/v1.3.2/`](docs/contract/v1.3.2/) — Versioned v1.3.2 public contract

## Community

MemAuthority recognizes and supports the [LINUX DO](https://linux.do/) community.

## License

Released under the Apache License 2.0. See [`LICENSE`](LICENSE) for details.

Attribution and third-party notices are documented in [`NOTICE`](NOTICE) and [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
