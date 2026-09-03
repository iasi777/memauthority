# MemAuthority 接入指南

[English](ONBOARDING.md)

> 用途：MemAuthority 用户接入指南。
> 精确 API、schema 和安全行为以版本化公共契约 [`contract/v1.3.2/`](contract/v1.3.2/) 为准。

这份文档只回答三个问题：

1. 从零开始怎么建库；
2. 已有 `MEMORY.md` 或旧记忆库怎么迁移；
3. 建好以后怎么接入 Agent 并开始日常使用。

---

## 1. 开始前只需要理解一个分工

- **你负责把关**：在关键节点决定什么值得长期留下；
- **Agent 负责执行**：理解、检索、整理、迁移、更新和清理记忆；
- **MemAuthority 负责兜底**：让长期记忆可靠保存、按需读取，并保护写入状态。

> **你做少量高层裁决，具体整理交给 Agent。**

MemAuthority 不是自动收集聊天记录的工具，也不要求你先学会整套 Markdown schema 才能使用。

---

## 2. 选择你的入口

### 路径 A：从零开始

适合还没有值得迁移的长期 Agent Memory。

### 路径 B：导入已有记忆库

适合已经拥有：

- `MEMORY.md`；
- Prompt 文件；
- 项目交接；
- Markdown / 文本笔记；
- JSON 导出；
- 旧 Agent Memory；
- 聊天总结；
- 多份互相冲突的历史记忆。

对于 MemAuthority 的目标用户，第二条路径往往才是采用它的主要原因。

---

## 3. 从零开始

### 3.1 创建空 Vault

```sh
memauthority init ./vault
```

这会建立一个空的 Git-backed Vault，并创建初始 commit。

它不会自动生成示例项目，也不会替你猜应该记什么。

### 3.2 让 Agent 建立最小可用记忆

第一次完整建库，推荐让能够访问本地文件和 Git 的 Agent 在 managed `serve` 启动前进行 detached authoring。

给 Agent：

- [`AGENT-GUIDE_ZH.md`](AGENT-GUIDE_ZH.md)；
- 当前项目代码、文档和必要上下文；
- 当前已经值得带给未来 Agent 的项目事实、决定、进展或踩坑。

Agent 应只建立当前已经有理由存在的内容。

不要为了“看起来完整”预填所有 role。

`handoff` 是必须的，其他 role 可以随着真实工作自然出现。

### 3.3 验证

```sh
memauthority validate --json ./vault
```

Validation 检查：

- Vault 结构；
- role/frontmatter；
- path 和 symlink 安全；
- 日期和跨文件同步；
- UTF-8 / source 限制；
- high-confidence secret。

Validation **不会**判断内容是否聪明、是否真的重要、是否已经语义过时。

结构通过以后，仍应按需审核 active memory 本身。

### 3.4 提交并启动 managed service

确认结果以后提交 Git，再启动：

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

注意：

- 去掉 `--write-enabled` 即为只读；
- `state-dir` 必须位于 Vault Authority 之外；
- managed admission 需要有效、已提交、干净的 Vault；
- detached 直接编辑与 managed runtime ownership 不应同时进行。

---

## 4. 导入已有记忆库

这一方案已经定案：

> **导入由 Agent 做语义迁移，不由 MemAuthority 为每种格式开发 importer。**

### 4.1 为什么由 Agent 导入

旧记忆库真正困难的不是文件格式，而是语义：

- 哪些内容仍然有效；
- 哪些内容互相冲突；
- 哪些只是历史；
- 哪些涉及隐私；
- 哪些是当前状态、长期规则、阶段进展、踩坑或 TODO；
- 哪些应该彻底退出 active memory。

这些判断应该由理解任务和用户意图的 Agent 完成。

> **只要 Agent 能访问并理解旧内容，它就可以迁移。**

### 4.2 正确顺序

Agent 必须先理解 MemAuthority，再读取旧库：

1. 阅读 [`AGENT-GUIDE_ZH.md`](AGENT-GUIDE_ZH.md)；
2. 理解 Context / Memory / TODO / Forget；
3. 理解四种 role；
4. 理解当前真相应收敛，而不是保留完整更正历史；
5. 再读取旧资料；
6. 去重、合并、更新、分类和淘汰；
7. 输出符合 MemAuthority 结构的结果。

> **Format-agnostic at the Agent layer; deterministic at the MemAuthority layer.**

### 4.3 导入是蒸馏，不是复制

不要因为旧系统曾经保存过某条内容，就默认继续保存。

每条旧内容都应该重新决定：

- 保留为当前 Memory；
- 与其他内容合并；
- 更新成较新的当前真相；
- 转成 TODO；
- 留在原始文档，只在 MemAuthority 放结论或指针；
- 从 active memory 中丢弃。

一份 5000 行旧库最后只剩 500 行，完全可能是一场更成功的迁移。

### 4.4 推荐的用户指令

```text
先了解 MemAuthority 的记忆规则，再检查这份旧记忆库。
把仍值得长期保留的内容去重、合并、更新并重新组织；
过时、重复、纯历史、临时和不该长期保存的内容不要导入。
先给我迁移候选，我确认后再写入。
```

如果你信任来源和 Agent，也可以明确授权它直接迁移。

候选 review 是推荐工作流，不是服务器强制步骤。

### 4.5 旧内容到 MemAuthority 的常见映射

- 当前接手状态 → `handoff`
- 长期决定和约束 → `rules`
- 仍能解释当前状态的阶段进展 → `progress`
- 未来仍可能复现的失败模式 → `pitfalls`
- 以后值得恢复的未完成意图 → handoff TODO
- 完整设计文档和参考资料 → 留在原位置，按需保留简短结论或指针
- 过时、重复、临时、私密、纯历史内容 → 不进入 active memory

这是指导，不是服务器自动分类规则。

### 4.6 v1 大规模迁移流程

```text
memauthority init
  -> 停止或不要启动 managed ownership
  -> Agent 阅读 MemAuthority 规则
  -> Agent 读取并蒸馏旧库
  -> detached authoring 构建 canonical Vault
  -> memauthority validate --json
  -> 用户按需审核
  -> Git commit
  -> managed serve
```

当前 v1 的 `memory_create_project` 只创建最小 handoff scaffold；managed handoff mutation 也不能任意插入新的 H2 section。

因此，丰富的首次 handoff 或大量旧库迁移使用 detached authoring 是明确的 v1 路径，不是临时绕过方案。

### 4.7 后续增量迁移

Managed service 启动后，剩余零散旧内容可以由 Agent 按正常 typed mutation 逐步吸收。

如果需要大规模重构整个 Vault：

1. 明确停止 managed ownership；
2. 进行 detached authoring；
3. validate；
4. review；
5. commit；
6. 重新启动 managed service。

不要在 managed ownership 期间并发直接修改文件或 Git。

---

## 5. 怎么接到 Agent

MemAuthority 的日常 managed 使用通过 MCP。

本地最简单的是 stdio，由 MCP client 直接启动：

```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

不同 Agent / 客户端的配置格式不同，但本质相同：

- command：`memauthority`
- arguments：`serve` 和后续参数

通用接入原则和配置结构见 [`MCP-CONFIG_ZH.md`](MCP-CONFIG_ZH.md)。

连接建立后，Agent 会自动获得：

- tool descriptions；
- input schemas；
- annotations；
- resource metadata。

这些已经包含当前服务实际暴露的字段、机械约束和使用提示。

首次建库或旧记忆迁移时，**不要**因为 MemAuthority 能表达 runtime/environment 就顺手创建这类元数据。没有已登记 runtime 时，这些工具默认隐藏。普通单机项目建议一直关闭；只有多环境或部署拓扑确实值得长期记住时再开启。

普通 managed 使用不需要你每次重新解释参数，也不要脱离当前 tool schema 硬编码某个版本的字段。

[`AGENT-GUIDE_ZH.md`](AGENT-GUIDE_ZH.md) 补充的是跨工具原则，尤其适合：

- detached bootstrap；
- 旧库迁移；
- Agent 不确定 role 选择；
- Agent 不确定如何做低成本任务记录或后续维护。

### HTTP

HTTP 远程接入前，先阅读：

- [`../SECURITY_ZH.md`](../SECURITY_ZH.md)
- [`contract/v1.3.2/transport-auth.md`](contract/v1.3.2/transport-auth.md)

不要把本地 stdio 示例直接改成公网监听。

---

## 6. 第一次日常使用

### 6.1 按需 Recall

仅在当前会话确实缺少这个项目的背景时使用：

```text
先看看这个项目的 MemAuthority，需要什么再读什么。
```

没有固定的 Recall 顺序。Agent 应根据当前已经知道的信息选择最短路径：

- 当前上下文已经足够 → 不调用 MemAuthority；
- 已知明确资源 → 直接读取；
- 已知项目但不知道位置 → 在项目内检索；
- 需要快速接手项目整体状态 → 再优先考虑 `handoff`；
- Search result 只是坐标，只继续读取真正相关的 section。

### 6.2 精确记录

```text
把这次任务里值得长期记住的内容列出来，我决定哪些写入。
```

### 6.3 低精力记录

```text
记录一下这次任务。
```

Agent 默认保守写 `progress`，不复制聊天，不擅自升级长期规则。

### 6.4 任务结束后顺手维护

```text
检查一下这次任务实际用到的 MemAuthority 记忆，根据刚刚发生和核验的事实更新它，并清理过时内容。
```

刚完成任务的 Agent 掌握最新事实。默认只维护本次真正读取和使用过的 Memory，不必每次扫描整个 Vault。

### 6.5 TODO

```text
这个方向以后值得研究，先放到 MemAuthority TODO，别占现在的上下文。
```

完成后，TODO 通常删除；长期结论另行进入 Memory。

---

## 7. 首次使用检查清单

- [ ] Vault 已由 `memauthority init` 创建；
- [ ] 首次完整建库或旧库迁移发生在 detached authoring 阶段；
- [ ] `memauthority validate --json` 通过；
- [ ] 用户按需审核 active memory，而不只看 validation；
- [ ] Vault 已提交且 worktree clean；
- [ ] `state-dir` 位于 Vault 之外；
- [ ] managed service 按需要启用或关闭写入；
- [ ] MCP client 已正确配置；
- [ ] HTTP 暴露前已完成安全配置；
- [ ] Agent 知道按需 Recall、保守记录、任务后维护和当前真相收敛原则。

---

## 8. 不要这样做

- 不要把完整聊天历史批量复制进 MemAuthority；
- 不要要求用户手工把旧库翻译成四个 role；
- 不要把 validation 当语义质量证明；
- 不要在 managed ownership 活跃时直接编辑 Vault；
- 不要在每个任务开始时加载全部 role；
- 不要把 checked TODO 当永久历史；
- 不要暗示删除 active memory 已经擦除 Git history；
- 不要把旧库导入重新设计成 server-side generic importer。
