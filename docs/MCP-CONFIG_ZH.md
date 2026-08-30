# MemAuthority MCP 接入

[English](MCP-CONFIG.md)

这份文档说明稳定的接入原则，不尝试维护所有 MCP 客户端各自不断变化的配置 UI。

精确 CLI 和 transport / auth 行为以版本化公共契约 [`contract/v1.3.1/`](contract/v1.3.1/) 为准。

---

## 最简单的方式：本地 stdio

MemAuthority `serve` 默认使用 stdio。

只读：

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state
```

可写：

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

注意：

- `--vault` 和 `--state-dir` 都是必需的；
- `state-dir` 必须位于 Vault Authority 之外；
- 写入默认关闭；
- managed admission 需要有效、已提交、干净的 Vault；
- detached 直接编辑与 managed ownership 不应同时进行。

---

## MCP client 里实际需要表达什么

不同客户端的字段名字不同，但本质只是告诉客户端：

- command：`memauthority`
- args：`serve`、`--vault`、Vault 路径、`--state-dir`、state 路径；
- 如果需要写入，再加 `--write-enabled`。

例如，很多客户端的配置概念上类似：

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

这只是**结构示意**，不是所有 MCP 客户端都能原样粘贴的通用配置。

客户端自己的配置文件位置、server key 名称和 UI 入口，请使用该客户端当前版本的官方说明。

---

## 可选的 Runtime Metadata

如果 Vault 里没有已登记的 `runtime_resource`，runtime metadata 会**默认关闭**。大多数只有一台电脑、一个工作副本的用户都不需要它。保持关闭可以缩小 MCP 工具面，也能避免 Agent 在首次建库或旧记忆迁移时凭空创造部署拓扑。

只有当“项目在哪些工作/运行环境、如何部署”这类事实确实值得跨会话长期保留时，才显式开启：

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled \
  --runtime-enabled
```

兼容行为是保守的：如果 Vault 已经有已登记 runtime resource，即使没有这个 flag，MemAuthority 也会自动继续暴露 runtime 工具。

Runtime ID 不再绑定维护者自己的机器。`host_id` 是可选的用户自定义 ID；只有一个 environment 时可以省略 `environment_id`，MemAuthority 会规范化为 `default`；有多个 environment 时再显式给每个环境稳定命名。普通单机用户不要因为看见 `host_id` 字段就强行填写。

---

## Agent 连接后还需要额外塞一大段 MemAuthority Prompt 吗？

通常不需要。

Managed MCP 会提供当前服务的：

- concise server instructions；
- tool descriptions；
- input schemas；
- annotations；
- resource metadata。

[`AGENT-GUIDE_ZH.md`](AGENT-GUIDE_ZH.md) 只补充跨工具 doctrine，例如 progressive Recall、保守记录、CAS conflict 后重新读取、任务后维护和旧库语义迁移。

如果某个字段或 operation 对任务很关键，让 Agent 以**当前连接实际返回的 schema**和**对应 release contract**为准，不要从旧 prompt 猜参数。

---

## HTTP 不是“把 stdio 命令换个地址”

HTTP 必须显式使用：

```text
--transport http
```

frozen transport contract 的基本安全边界包括：

- 默认监听 `127.0.0.1:8000`；
- 没有 OAuth 时只能监听 loopback；
- non-loopback HTTP 必须启用 OAuth；
- write-enabled HTTP 必须启用 OAuth；
- OAuth state 必须位于 Vault Authority 之外。

生产暴露前请阅读：

- [`../SECURITY_ZH.md`](../SECURITY_ZH.md)
- [`contract/v1.3.1/transport-auth.md`](contract/v1.3.1/transport-auth.md)

不要为了让部署“先跑起来”而绕过这些拒绝规则。

---

## 多实例 / fencing

单机本地 stdio 通常不需要先理解 fencing。

如果部署要求 single-primary fencing，`serve` 提供对应的 primary / node 配置；这属于部署安全边界，不建议从普通本地示例复制猜测。

精确选项请查看当前 release 的：

```sh
memauthority serve --help
```

并对照当前的 [`contract/v1.3.1/managed-runtime.md`](contract/v1.3.1/managed-runtime.md)，或已安装版本对应的冻结契约。

---

## 接入后的第一次验证

连接成功后，不要一上来读取整个 Vault。

仅在当前会话确实缺少这个项目的背景时，可以直接对 Agent 说：

```text
先看看这个项目的 MemAuthority，需要什么再读什么。
```

理想行为不是固定 `route → handoff`：当前 Context 已经足够时不需要调用 MemAuthority；项目不确定时 route，已知 URI 时直接 read，已知项目但不知道位置时 search，需要恢复整体项目状态时再优先考虑 handoff。
