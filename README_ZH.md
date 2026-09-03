# MemAuthority — 面向 AI Agent 的 Git 长期记忆系统

[English](README.md)

MemAuthority 是一套面向 AI Agent 的 Git-backed 的长期记忆系统，通过 MCP 对外提供能力。它让新的会话、另一台机器或其他 Agent 获得项目已经积累的关键上下文，继续工作，而不是再次摸索同样的决定和踩坑

它不追求巨细靡遗地记住所有事情，而是专注于沉淀：

> **未来 Agent 不应再重复摸索的经验与结论**

清晰的分工：
- **你**负责把关：判断哪些信息真正值得长期留下；
- **Agent** 负责执行：理解、检索、归纳、更新与清理记忆；
- **MemAuthority** 负责兜底：让长期记忆可靠保存、按需读取，并防止旧版本覆盖、重复重试和异常中断破坏记忆

内容意味着什么、该不该修改，仍然由 Agent 判断；MemAuthority 负责让最终留下的记忆内容长期可靠

---

## 它适合谁？

MemAuthority 适合更看重长期记忆质量的用户

你可能正在经历各类平台自带 Memory 的不可控感：
- 不清楚为什么会被记下；
- 不确定何时会被召回使用；
- 无法确认内容是否已经过时；
- 想要精确修改或删除时无从下手

你也可能尝试过维护 `MEMORY.md`，但随着内容积累，它变得越来越臃肿、杂乱，甚至开始干扰 Agent 的正常判断

MemAuthority 适合的场景：

> **愿意为记忆质量投入少许精力把关，但希望将具体整理工作全部交给 Agent**

这里的“花精力”并不需要你频繁手动修改文件，而是在关键节点做几个有价值的高层裁决：
- 某些信息是否值得长期保留；
- 什么时候应该整理记忆；
- 哪些内容涉及隐私或敏感信息；
- Agent 提议整理的结果是否符合你的真实意图

其余机械繁琐的格式整理、分类归档与检索调用，统统交由 Agent 处理

---

## 为什么不直接用 MEMORY.md？

如果你的项目记忆体量较小、内容极少变动，或者你不想在记忆维护上耗费任何注意力，继续使用 `MEMORY.md` 是最省心的选择

当这份文件不再只是一份笔记，而开始成为未来会话必须信任的长期项目状态时，MemAuthority 才真正体现价值

它聚焦于解决更高阶的需求：

> **把“维护长期记忆”本身，变成一套可靠、可控的工程化工作流**

它带来的不只是“让 Agent 编辑 Markdown”，而是一整套严谨的运行机制：
- **按需精准读取**：仅在任务真正需要时才加载相关记忆，避免无关信息撑爆或污染上下文；
- **清晰的角色分层**：明确区分当前接手状态、长期规则、阶段进展与避坑经验；
- **并发与版本安全**：修改前严格校验版本（CAS），防止旧 revision 静默覆盖新内容；
- **幂等防重保障**：网络波动或执行中断时的重试操作，不易产生重复记忆；
- **收敛与历史追溯**：活跃记忆库可以持续精简收敛，而完整的修改历史始终由 Git 负责追溯；
- **事务与容灾恢复**：写入中断或异常时具备明确的事务日志（Journal）与恢复机制

可以把它简单理解为：

> **Authority 保存未来 Agent 现在应该继承的状态；Git 保存完整历史**

一句话概括：

> **`MEMORY.md` 优化的是简单直接；MemAuthority 优化的是项目变复杂之后，长期记忆仍然能够可靠维护**

---

## 平时怎么用？

在理想的工作流中，MemAuthority 隐身于 Agent 背后。你平时只需要用简短自然的指令与 Agent 交互：

### 1. 开始任务时按需读取
*仅在当前对话确实缺少该项目背景时使用：*
> **“先看看这个项目的 MemAuthority，需要什么再读什么”**

Agent 应该选择最短、最合适的读取路径：已知具体位置就直接读取，不知道位置时再检索，需要快速接手项目整体状态时再看 `handoff`。如果当前对话本身已经有足够上下文，就完全没有必要调用 MemAuthority

### 2. 精确把关要记录的内容
> **“把这次任务里值得长期记住的内容列出来，我来决定写哪些”**

Agent 提炼候选条目，由你最终决定保留、修改还是舍弃

### 3. 省心快速记录任务
> **“记录一下这次任务”**

这也是常见用法。默认情况下，Agent 会采用保守策略，仅将其作为一次低风险的 `progress`（阶段进展）记录，而不会擅自改动长期 `rules`（规则）

### 4. 任务结束后顺手维护
> **“检查一下这次任务实际用到的 MemAuthority 记忆，根据刚刚发生和核验的事实更新它，并清理过时内容”**

刚完成任务的 Agent 掌握最新的代码、工具结果、运行事实和用户裁决。它只需要维护本次真正读取和使用过的记忆，不必每次扫描整个 Vault

### 5. 暂存延后想法，释放当前上下文
> **“这个方向以后值得研究，先放到 MemAuthority TODO，别占现在的上下文”**

这里的 TODO 不是项目管理工具，而是：
> **值得未来处理，但眼下不需要占据思考带宽的信息暂存点**

后续处理完成后，TODO 本身通常应当删除；真正形成的长期有效结论，再单独归入长期记忆

---

## “记录一下这次任务”，怎么避免越记越乱？

MemAuthority 不强制要求你每次记录前都先思考“该放进 rules 还是 progress”

当你发出类似“记录一下这次任务”的模糊指令时，Agent 的默认行为应当是：
- **优先写入 `progress`**；
- 仅记录本次任务实际交付的核心成果；
- 仅记录有意义的状态演进；
- 仅记录未来继续工作时仍需了解的关键信息

**默认应当主动过滤的内容：**
- 完整的聊天流水与客套话；
- 琐碎的操作级流水与尝试细节（operation-by-operation）；
- 没有未来复用价值的临时失败过程；
- 库中已经存在的重复事实；
- 临时、过时、私密或纯推测性的内容

同时，Agent 不应仅仅因为某句话“听起来很重要”，就擅自将其提升为长期 `rules` 真正稳固的规则、接手状态和踩坑教训，应在长期的实际工作中逐步沉淀

**MemAuthority 底层提供系统级兜底保护：**
- 写入内容必须遵循规范的固定结构；
- 不同记忆角色（Role）只能通过允许的方式进行修改；
- 自动拦截高置信度的 Secret / 敏感凭证；
- 幂等机制防止重试变为重复堆积；
- 版本校验（CAS）机制，旧 revision 无法静默覆盖新 revision；
- 写入前对候选 Vault 内容进行合法性验证；
- 写入中断时具备 Journal 事务与恢复支持

系统的承诺并不是“永远不产生一条低质量记录”，而是：

> **由 Agent 执行保守记录，由 MemAuthority 保障底层状态可靠，再通过后续真实工作推动记忆持续精炼收敛**

这里的“一致性”分为两层：
- **语义一致性**：由 Agent 基于最新任务事实与既有记忆进行梳理维护；
- **状态一致性**：由 MemAuthority 通过 Git Authority、Revision CAS、幂等性和事务机制提供底层保障

记住：`progress` 是低成本的阶段性记录入口，而不是永久不变的归档日志

---

## 我已经有 MEMORY.md 或旧记忆库怎么办？

无需手动逐行重写

针对旧库迁移，MemAuthority 采用明确的迁移方案：

> **由 Agent 负责语义迁移，而不是由 MemAuthority 为每种旧格式编写专门的 Importer**

只要 Agent 能够读取并理解你的旧内容，就能支持各类迁移来源：
- 现有的 `MEMORY.md`；
- 通用 Markdown 或纯文本文档；
- JSON 导出文件；
- Prompt 提示词文件；
- 历史交接文档与聊天总结；
- 多份存在冲突的旧笔记；
- 其他任何 Agent 可理解的文本内容

**标准迁移流程：**
1. Agent 先学习 MemAuthority 接收的长期记忆标准与结构规范；
2. Agent 读取旧资料库；
3. 执行去重、合并、更新与分类整理；
4. 剔除过时、冗余、纯流水账、临时性以及不宜长期保存的内容；
5. 将提炼后的最终内容输出为标准的 MemAuthority 结构

你可以直接对 Agent 这样说：

> **“先了解 MemAuthority 的记忆规范，然后检查这份旧记忆库 把仍值得长期保留的内容进行去重、合并、更新和重组；过时、重复、纯流水账、临时和不该长期保存的内容全部剔除 先整理出迁移候选方案，由我确认后再执行写入 ”**

迁移的本质是**重新进行一次记忆裁决**，而不是机械复制

一份 5000 行的庞杂旧库，最终精简为 500 行高质量记忆，往往是一次非常成功的迁移

针对首次完整建库或大规模旧库迁移，v1 推荐流程为：

```text
init（初始化空库）
  -> Agent 在 detached 模式下整理 Vault
  -> validate（校验合法性）
  -> 用户人工审核
  -> Git commit（提交版本）
  -> managed serve（启动受管服务）
```

完整操作流程请参阅 [`docs/ONBOARDING_ZH.md`](docs/ONBOARDING_ZH.md)

---

## MemAuthority 里存什么？

MemAuthority 中的长期记忆严格划分为四种角色（Roles）：

### `handoff`（接手状态）
当前接手该项目必须了解的**最小关键状态**
它应当保持简短、直接并随项目演进而持续刷新，切忌写成冗长的第二份 README

### `rules`（长期规则）
未来 Agent 开展工作仍需遵守的长期决定、架构约束与行为准则
只记录最终的裁决结果，不记录冗长的讨论与争辩过程

### `progress`（阶段进展）
对未来后续工作仍具参考价值的阶段性重要进展
它是最低成本的记录入口，但并非流水账式的永久日志

### `pitfalls`（避坑指南）
未来工作中仍有可能再次遭遇的典型失败模式与防范经验
并非每个普通报错都值得记录

一个行之有效的判断标准是：

> **未来的 Agent 知道这件事后，是否能够改变行动决策、省去大量重复探索，或者避免重蹈覆辙？**

如果不能，大概率不需要将其沉淀为长期记忆

---

## 为什么不把所有记忆都塞进 Context？

因为**记得多并不等于记得对**

MemAuthority 支持渐进式按需召回（Progressive Recall），但不要求 Agent 遵守一套固定仪式：
1. 当前对话已经有足够信息时，完全不调用 MemAuthority；
2. 项目不明确时，先定位并确认项目；
3. 已知明确 URI 或 Section 时，直接精确读取；
4. 已知项目但不知道具体位置时，再在项目范围内检索；
5. 需要快速恢复项目整体接手状态时，`handoff` 通常是最合适的起点；
6. Agent 自己判断结果是否相关，只继续读取真正有助于当前任务的内容

> **Search results are coordinates, not context.**
> （搜索结果是定位坐标，而不是直接塞进 Prompt 的上下文）

目标很简单：不要让无关记忆进入当前 Context，把“需要多少证据才够”留给正在执行任务的 Agent 判断

---

## 多个 Agent 会不会把记忆写乱？

MemAuthority v1 遵循一个简单原则：

> **Multi-Agent, Single Authority**（多 Agent 使用，单一权威源）

不同 Agent 或客户端可以轮流读取同一份 Managed Authority，并请求修改；MemAuthority 始终维持一条线性、确定的版本历史。v1 仍然是面向**单用户、单写者**的系统，而不是团队多人同时编辑的协作数据库

如果 Agent A 已经更新了记忆，而 Agent B 仍试图基于旧 revision 提交修改，MemAuthority 会明确返回 conflict 并拒绝写入，而不是让旧状态静默覆盖新状态

此时 Agent B 需要重新读取最新内容，再根据实际情况决定合并、重写、放弃还是询问用户

MemAuthority 不负责判断双方哪一个主观观点更正确，它只保证一件更确定的事：

> **让并发分歧明确暴露，绝不让冲突悄悄变成错误的 Authority**

---

## 当前 Memory 不是历史档案

Git 负责忠实记录 Authority 的完整演变历史，随时可以追溯记忆；Agent 仅读取活跃的 Memory 里**当下对未来 Agent 仍有价值的内容**

- 规则调整了，就直接更新规则；
- 状态演进了，就同步刷新状态；
- 内容过时且不再需要召回，就让其退出当前活跃记忆

> **历史负责记录演进，记忆负责保持收敛**

将内容从当前 Memory 中移除，并不代表历史记录从 Git 中物理抹除 但是请注意：真正不适宜进入记忆库的敏感凭据或临时琐碎信息，最理想的做法是在初次写入前就主动排除

---

## 安装

MemAuthority 依赖 Git，并要求 Go 1.26.5 或更高版本。安装当前稳定版本：

```sh
go install github.com/iasi777/v-memory/cmd/memauthority@v1.3.1
```

v1.x 的 Go module identity 为了兼容性继续保持 `github.com/iasi777/v-memory`，即使 canonical repository 和产品名已经变为 MemAuthority。GitHub 会把旧仓库地址重定向到 `iasi777/memauthority`

验证：

```sh
memauthority version
```

预期输出：

```text
memauthority 1.3.1
```

已有自动化可以继续安装并调用兼容命令：

```sh
go install github.com/iasi777/v-memory/cmd/v-memory@v1.3.1
v-memory version
```

目前没有发布预编译二进制。如果需要从源码构建：

```sh
go build -trimpath -o ./memauthority ./cmd/memauthority
./memauthority version
```

Release CI 会在 Linux、macOS、Windows 原生 runner 上运行测试、vet 和 CLI 构建；Linux CI 另外验证生产使用的 Linux/ARM64 目标

---

## 从零开始

### 1. 创建空 Vault
```sh
memauthority init ./vault
```

### 2. 初始化与整理
首次完整建库或执行大规模旧库迁移时，请让具备文件与 Git 访问权限的 Agent 先阅读：
[`docs/AGENT-GUIDE_ZH.md`](docs/AGENT-GUIDE_ZH.md)

整理完成后执行校验：
```sh
memauthority validate ./vault
```

### 3. 提交并启动服务
审核内容并提交至 Git 后，通过 MCP 启动托管服务（Managed Service）：
```sh
memauthority serve \
  --vault /absolute/path/to/vault \
  --state-dir /absolute/path/to/state \
  --write-enabled
```

- 省略 `--write-enabled` 即以**只读模式**运行；
- `state-dir` 必须存放在 Vault Authority 目录之外；
- 请勿同时进行 Detached 模式（直接编辑文件）与 Managed 模式（服务运行中托管）；
- 可选的 declarative runtime metadata 在 Vault 没有已登记 runtime 时**默认关闭**。绝大多数单机用户建议保持关闭。已有 `runtime_resource` 的 Vault 会自动继续暴露 runtime 工具；只有你明确需要开始记录工作环境/部署拓扑时，才添加 `--runtime-enabled`

本地接入最简单的方式是让 MCP 客户端通过 stdio 直接拉起上述命令

连接建立后，MemAuthority 会自动向 Agent 提供 Tool 描述、Input Schema、Annotations 和 Resource 元数据 在日常受管使用中，你无需每次手动向 Agent 解释参数规则

[`AGENT-GUIDE.md`](docs/AGENT-GUIDE_ZH.md) 补充了跨工具维度的使用原则（如按需召回、保守记录、角色选择与旧库迁移等）

如需通过 HTTP 远程接入，请务必先查阅 [`SECURITY_ZH.md`](SECURITY_ZH.md) 以及冻结的 v1.3.1 Transport / Auth 规范

---

## MemAuthority 不是什么

如果你的主要诉求属于以下类型，其他专门工具往往更加合适：
- **完全自动记录个人日常偏好**：更适合使用各模型/平台自带的 Memory；
- **体量极小、内容稳定且乐于手动维护**：直接使用 `MEMORY.md`；
- **在海量技术文档中检索知识**：更适合使用标准 RAG / 文档检索引擎；
- **供人类阅读的完整项目文档**：应编写专用的 README、Wiki、ADR 或 Docs；
- **追踪排期、负责人、Deadline 与协作依赖**：应使用专业的任务或项目管理工具（如 Jira、Linear 等）

MemAuthority 不是聊天日志归档，不是全量知识库，也不是任务管理系统

它唯一专注保留的是：

> **未来 Agent 不应再重复摸索的经验与结论**

---

## 构建与验证

```sh
go test ./...
go vet ./...
go mod verify
go build -trimpath -o ./memauthority ./cmd/memauthority
```

---

## 公共契约

当前公共兼容性基线为 **v1.3.1**；此前已经发布的历史契约快照保持不变

Vault 存储格式、MCP 工具、Managed 运行时行为、变更/拒绝规则、安全边界、Transport / Auth 行为以及兼容策略的权威定义，统一维护在 [`docs/contract/v1.3.1/`](docs/contract/v1.3.1/) 下

*本 README 与其他用户文档用于辅助理解；细节不一致时，以对应版本的公共契约为准*

## Version

```sh
memauthority version
memauthority --version
```
**AI Agent Memory**、**Long-Term Memory**、**MCP Memory Server**、**Git-backed Memory**、**Agent Memory Infrastructure**、**Agent Continuity**

v1.3.1 中，主命令输出 `memauthority 1.3.1`；兼容命令输出 `v-memory 1.3.1`

## Security

在将服务暴露至 HTTP Transport 之前，请务必阅读 [`SECURITY_ZH.md`](SECURITY_ZH.md)

- 写入权限默认关闭，需显式添加 `--write-enabled` 标志开启；
- HTTP 写入必须严格遵守冻结 v1 规范中的 OAuth 与安全要求；
- MemAuthority 内置高置信度 Secret 敏感信息扫描，但它不能替代全功能的隐私检测工具

## 相关文档

- [`docs/ONBOARDING_ZH.md`](docs/ONBOARDING_ZH.md) — 快速起步、旧库迁移与首次接入指南
- [`docs/MCP-CONFIG_ZH.md`](docs/MCP-CONFIG_ZH.md) — stdio / HTTP 接入原则与配置格式规范
- [`docs/AGENT-GUIDE_ZH.md`](docs/AGENT-GUIDE_ZH.md) — 供 Agent 遵循的召回、记录、维护与迁移准则
- [`docs/FAQ_ZH.md`](docs/FAQ_ZH.md) — 常见问题、设计边界与取舍考量
- [`examples/README_ZH.md`](examples/README_ZH.md) — 可运行的示例 Vault 与首次本地 MCP 会话
- [`SECURITY_ZH.md`](SECURITY_ZH.md) — 安全支持范围、部署边界与私密报告入口
- [`docs/contract/v1.3.1/`](docs/contract/v1.3.1/) — v1.3.1 版本化公共契约

## 社区

MemAuthority 认可并支持 [LINUX DO](https://linux.do/) 社区。

## License

本项目基于 Apache License 2.0 开源，详情请参阅 [`LICENSE`](LICENSE)

归属与第三方说明请参阅 [`NOTICE`](NOTICE) 和 [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)
