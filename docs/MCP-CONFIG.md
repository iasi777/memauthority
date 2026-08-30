# MemAuthority MCP Connection Guide

[中文](MCP-CONFIG_ZH.md)

This guide documents stable connection principles rather than trying to track every MCP client's changing UI.

For exact CLI and transport / authentication behavior, use the versioned public contract under [`contract/v1.3/`](contract/v1.3/).

---

## Simplest Setup: Local stdio

`memauthority serve` uses stdio by default.

Read-only:

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state
```

Read-write:

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

Important:

- both `--vault` and `--state-dir` are required;
- `state-dir` must be outside Vault Authority;
- writes are disabled by default;
- Managed admission requires a valid, committed, clean Vault;
- detached direct editing and Managed ownership must not run concurrently.

---

## What an MCP Client Actually Needs

Client field names vary, but the essential configuration is only:

- command: `memauthority`;
- arguments: `serve`, `--vault`, the Vault path, `--state-dir`, and the state path;
- add `--write-enabled` only when writes are required.

Many clients use a structure conceptually similar to:

```json
{
  "command": "memauthority",
  "args": [
    "serve",
    "--vault", "/absolute/path/to/vault",
    "--state-dir", "/absolute/path/to/state",
    "--write-enabled"
  ]
}
```

This is a structural example, not a universal copy-paste configuration for every MCP client.

Use the current official documentation for your client to find its configuration file, server key, or UI entry point.

---

## Optional Runtime Metadata

Runtime metadata is intentionally **off by default** for a Vault that has no registered `runtime_resource`. Most users with one computer and one working copy do not need it. Leaving it off keeps the MCP surface smaller and avoids inventing deployment topology during onboarding or legacy-memory import.

Enable it only when durable environment/deployment facts materially help future agents:

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled \
  --runtime-enabled
```

Compatibility behavior is conservative: if the Vault already contains a registered runtime resource, MemAuthority automatically exposes the runtime tools even without the flag.

Runtime identifiers are no longer tied to maintainer machines. `host_id` is optional and user-defined. For a single environment, `environment_id` may be omitted and is canonicalized to `default`; with multiple environments, give each one an explicit stable `environment_id`. A typical single-machine user should not add `host_id` merely because the field exists.

---

## Do I Need to Add a Large MemAuthority Prompt After Connecting?

Usually not.

The Managed MCP server already exposes its current:

- concise server instructions;
- tool descriptions;
- input schemas;
- annotations;
- resource metadata.

[`AGENT-GUIDE.md`](AGENT-GUIDE.md) adds only the cross-tool doctrine that schemas do not express well: progressive recall, conservative recording, rereading after CAS conflicts, post-task maintenance, and semantic migration of legacy memory.

When a field or operation is critical, the agent should rely on the schema returned by the current connection and the contract for that release, not guess from an old prompt.

---

## HTTP Is Not “stdio at a URL”

HTTP must be selected explicitly:

```text
--transport http
```

Core safety boundaries in the frozen transport contract include:

- default listen address is `127.0.0.1:8000`;
- without OAuth, HTTP may listen only on loopback;
- non-loopback HTTP requires OAuth;
- write-enabled HTTP requires OAuth;
- OAuth state must live outside Vault Authority.

Before exposing production HTTP, read:

- [`../SECURITY.md`](../SECURITY.md)
- [`contract/v1.3/transport-auth.md`](contract/v1.3/transport-auth.md)

Do not bypass refusal rules merely to make a deployment “start working.”

---

## Multiple Instances and Fencing

A normal single-machine stdio setup does not require you to understand fencing first.

When a deployment requires single-primary fencing, `serve` exposes the corresponding primary / node configuration. This is a deployment-safety boundary and should not be guessed by copying a basic local example.

Check the exact options for the installed release:

```sh
memauthority serve --help
```

Then compare them with the current [`contract/v1.3/managed-runtime.md`](contract/v1.3/managed-runtime.md) contract or the frozen contract for the installed release.

---

## First Check After Connecting

Do not begin by reading the entire Vault.

Only when the current session genuinely lacks project context, you can tell the agent:

```text
Check this project's MemAuthority and read only what you actually need.
```

The ideal behavior is not a fixed `route -> handoff` ritual. If current context is enough, no MemAuthority call is needed. Route when the project is uncertain, read directly when the URI is known, search when the project is known but the location is not, and prefer handoff when the task needs an overall project-state recovery.
