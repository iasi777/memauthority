# 可运行示例

[English](README.md)

本目录在 [`sample-vault/`](sample-vault/) 中提供一个经过脱敏、结构合法的示例 Vault。它包含四种记忆角色，但不包含 runtime 或生产环境元数据。

精确行为以当前 [`../docs/contract/v1.3.2/`](../docs/contract/v1.3.2/) 版本化公共契约为准。

## 1. 创建临时示例 Vault

安装或构建 `memauthority` 后，在仓库根目录执行：

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

校验结果应包含 `"valid": true`。后续使用的临时 state 目录与 `vault` 互为同级目录，因此位于 Vault Authority 之外。

## 2. 启动本地 MCP 服务

只读模式：

```sh
memauthority serve \
  --vault "$MEMAUTHORITY_DEMO_ROOT/vault" \
  --state-dir "$MEMAUTHORITY_DEMO_ROOT/state"
```

可写模式：

```sh
memauthority serve \
  --vault "$MEMAUTHORITY_DEMO_ROOT/vault" \
  --state-dir "$MEMAUTHORITY_DEMO_ROOT/state" \
  --write-enabled
```

在 MCP 客户端中使用相同的 command 和 arguments。很多客户端不会展开参数数组中的环境变量，因此配置时应替换为绝对路径。详见 [`../docs/MCP-CONFIG_ZH.md`](../docs/MCP-CONFIG_ZH.md)。

## 3. 尝试 Agent 工作流

客户端连接后，可以先说：

```text
先看看 demo 项目的 MemAuthority，需要什么再读什么。
```

Agent 可以把别名 `sample-project` 路由到 `demo` 项目，读取 `memory://projects/demo/handoff`，并按需检索 rules、progress 或 pitfalls。

然后尝试一次保守的 Managed 写入：

```text
记录示例 MCP 连接已经验证成功，作为一条简短的 progress。
```

可写服务应通过受控 mutation 路径完成 Git 版本提交，并保持 Vault 合法。

## 安全说明

- 示例不包含凭据或真实生产路径。
- 不要直接把示例当作自己项目的长期记忆。
- Managed 服务运行时不要直接编辑 Vault 文件。
- HTTP / OAuth 暴露前，请阅读 [`../SECURITY_ZH.md`](../SECURITY_ZH.md) 和当前的 [Transport / Auth 契约](../docs/contract/v1.3.2/transport-auth.md)。
