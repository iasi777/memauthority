# Runnable Example

[中文](README_ZH.md)

This directory contains a sanitized, structurally valid sample Vault under [`sample-vault/`](sample-vault/). It demonstrates all four memory roles without runtime or production metadata.

Exact behavior is defined by the current versioned public contract under [`../docs/contract/v1.3.1/`](../docs/contract/v1.3.1/).

## 1. Create a Temporary Demo Vault

Run these commands from the repository root after installing or building `memauthority`:

```sh
MEMAUTHORITY_DEMO_ROOT="$(mktemp -d)"
memauthority init "$MEMAUTHORITY_DEMO_ROOT/vault"
cp -R examples/sample-vault/. "$MEMAUTHORITY_DEMO_ROOT/vault/"
git -C "$MEMAUTHORITY_DEMO_ROOT/vault" add INDEX.yaml demo
git -C "$MEMAUTHORITY_DEMO_ROOT/vault" \
  -c user.name='MemAuthority Example' \
  -c user.email='example@memauthority.invalid' \
  commit -m 'example: add sample memory project'
memauthority validate --json "$MEMAUTHORITY_DEMO_ROOT/vault"
```

The validation result should contain `"valid": true`. The temporary state directory used below is a sibling of `vault`, so it remains outside Vault Authority.

## 2. Start a Local MCP Server

Read-only:

```sh
memauthority serve \
  --vault "$MEMAUTHORITY_DEMO_ROOT/vault" \
  --state-dir "$MEMAUTHORITY_DEMO_ROOT/state"
```

Writable:

```sh
memauthority serve \
  --vault "$MEMAUTHORITY_DEMO_ROOT/vault" \
  --state-dir "$MEMAUTHORITY_DEMO_ROOT/state" \
  --write-enabled
```

For an MCP client, use the same command and arguments in its server configuration. Replace shell variables with absolute paths because many clients do not expand environment variables in argument arrays. See [`../docs/MCP-CONFIG.md`](../docs/MCP-CONFIG.md).

## 3. Try the Agent Workflow

After the client connects, try:

```text
Check the demo project's MemAuthority and pull in only what you need.
```

The agent can route the alias `sample-project` to project `demo`, read `memory://projects/demo/handoff`, and selectively retrieve rules, progress, or pitfalls.

Then try a conservative managed write:

```text
Record that the sample MCP connection was verified. Keep it as concise progress.
```

The writable server should use the controlled mutation path, create a Git-backed revision, and leave the Vault valid.

## Safety Notes

- The sample contains no credentials or real production paths.
- Do not use the sample as a substitute for curating your own project memory.
- Do not edit Vault files directly while the Managed server is running.
- For HTTP/OAuth exposure, read [`../SECURITY.md`](../SECURITY.md) and the current [transport/auth contract](../docs/contract/v1.3.1/transport-auth.md).
