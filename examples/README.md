# Runnable Example

[中文](README_ZH.md)

This directory contains a sanitized, structurally valid sample Vault under [`sample-vault/`](sample-vault/). It demonstrates all four memory roles without runtime or production metadata.

Exact behavior is defined by the versioned public contract under [`../docs/contract/v1.1/`](../docs/contract/v1.1/).

## 1. Create a Temporary Demo Vault

Run these commands from the repository root after installing or building `v-memory`:

```sh
VM_DEMO_ROOT="$(mktemp -d)"
v-memory init "$VM_DEMO_ROOT/vault"
cp -R examples/sample-vault/. "$VM_DEMO_ROOT/vault/"
git -C "$VM_DEMO_ROOT/vault" add INDEX.yaml demo
git -C "$VM_DEMO_ROOT/vault" \
  -c user.name='V-Memory Example' \
  -c user.email='example@v-memory.invalid' \
  commit -m 'example: add sample memory project'
v-memory validate --json "$VM_DEMO_ROOT/vault"
```

The validation result should contain `"valid": true`. The temporary state directory used below is a sibling of `vault`, so it remains outside Vault Authority.

## 2. Start a Local MCP Server

Read-only:

```sh
v-memory serve \
  --vault "$VM_DEMO_ROOT/vault" \
  --state-dir "$VM_DEMO_ROOT/state"
```

Writable:

```sh
v-memory serve \
  --vault "$VM_DEMO_ROOT/vault" \
  --state-dir "$VM_DEMO_ROOT/state" \
  --write-enabled
```

For an MCP client, use the same command and arguments in its server configuration. Replace shell variables with absolute paths because many clients do not expand environment variables in argument arrays. See [`../docs/MCP-CONFIG.md`](../docs/MCP-CONFIG.md).

## 3. Try the Agent Workflow

After the client connects, try:

```text
Check the demo project's V-Memory and read only what you need.
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
- For HTTP/OAuth exposure, read [`../SECURITY.md`](../SECURITY.md) and the current [transport/auth contract](../docs/contract/v1.1/transport-auth.md).
