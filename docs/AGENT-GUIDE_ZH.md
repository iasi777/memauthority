# MemAuthority Agent 使用指南

[English](AGENT-GUIDE.md)

> 用途：MemAuthority Agent 操作指南。
> 精确 tool schema、Vault 格式和拒绝行为以版本化公共契约 [`contract/v1.3/`](contract/v1.3/) 为准。

这份指南不重复全部 MCP schema。

Managed MCP 连接后，你已经会自动获得 tool descriptions、input schemas、annotations 和 resource metadata。

本文只补充单个 tool schema 很难完整表达的**跨工具使用原则**，以及 detached bootstrap / 旧库迁移时需要遵守的 authoring doctrine。

---

## 1. 责任边界

- **用户**决定什么值得成为长期记忆。
- **你，Agent**负责理解、选择、搜索、压缩、合并、更新、淘汰和迁移。
- **MemAuthority**负责确定性 Recall、受控 mutation、revision safety 和 Git-backed Authority。

> **MemAuthority constrains state transitions, not intelligence.**

不要默认把对话历史当成长期 Memory。

---

## 2. 保留测试

只有当未来 Agent 知道某条内容会明显产生下面至少一种价值时，才考虑长期保存：

- 改变未来行为；
- 恢复必要项目状态；
- 避免重要的重新推理；
- 防止重复犯错。

使用四个去向：

- **Context**：只在当前任务需要；
- **Memory**：未来 Agent 仍应该知道；
- **TODO**：以后值得恢复，但现在不值得占用注意力；
- **Forget**：临时、私密、重复、过时、已消费或低价值内容。

> **不要最大化记忆数量，要最大化未来 Recall 的价值。**

---

## 3. 四种 role

### `handoff`

保存当前接手项目所需的最小工作集。

避免：

- 完整历史；
- 第二份 README；
- 已完成状态的长期堆积。

### `rules`

保存未来仍需遵守的长期决定和约束。

避免：

- 辩论流水；
- 已经被替代的旧规则；
- 临时偏好。

### `progress`

保存仍有未来价值的阶段进展。

它是低成本记录入口，不是永久操作日志。

### `pitfalls`

保存未来仍可能复现的失败模式和规避经验。

不要记录每个一次性错误。

### TODO

TODO 是 handoff 下精确 H2 `已知问题 / 待办` 中的顶层 checklist。

它表示 deferred intent，不是 deadline / owner / schedule 管理。

### 写出可维护的 section

每个 section 应尽量只表达一个可以独立 Recall、独立过时、独立更新的主题。

推荐：

- heading 直接说明主题；
- 第一段先写当前结论或状态；
- 只保留必要理由和行动影响；
- 不依赖原始对话才能理解；
- 多个无关决定拆成多个 section；
- 不在当前 Memory 中累积完整更正历史。

> **标题说明这是什么，第一句话说明现在什么是真的。**

---

## 4. Progressive Recall

不要在任务开始时机械读取 MemAuthority，更不要一次读取整个 Vault 或全部四个 role。

先判断当前 Context 是否已经足够完成任务；足够时，不调用 MemAuthority。需要长期项目状态时，再按当前信息状态选择最小动作：

- 项目 id / alias 不确定 → `memory_route`；
- 已知 URI 或明确 role / section → 直接 `memory_read`；
- 已知项目但不知道内容位置 → project-scoped `memory_search`；
- 需要接手项目整体状态时，`handoff` 通常是最有价值的首个资源；
- 只有明确探索跨项目经验时才使用 cross-project search；
- 只有任务需要时才继续展开。

> **Context first. Recall on demand.**

### Search results are coordinates, not context

`memory_search` hit 是候选位置，不是自动进入 Context 的指令。

对选中的结果，优先使用返回的：

- `resource_uri`；
- `line_start` / `line_end`；
- `resource_revision`。

推荐流程：

```text
search
  -> inspect candidates
  -> select relevant hit
  -> memory_read(uri + line range + expected_resource_revision)
```

如果 revision 冲突，重新读取当前内容，再重新推理。

不要机械地把 Search 返回的完整 heading path 当作 `memory_read` heading selector。

Cursor 只说明后面还有内容，不代表必须继续读完。

---

## 5. Managed write 原则

Managed ownership 活跃时，Authority mutation 必须通过 MemAuthority 提供的 mutation tools 完成，不要通过普通 machine / filesystem 工具直接修改 Vault 数据目录制造第二条写入路径。

需要大规模 detached authoring 时，先停止 managed ownership，再直接编辑、validate、review、commit，最后重新启动 managed service。

### 5.1 修改已有内容前先读取

需要 CAS 的 mutation 应使用 `memory_read` 返回的当前 revision。

遇到 conflict 时：

1. 不要盲目重试；
2. 重新读取；
3. 比较当前 Memory 与本次任务的新事实；
4. 决定合并、替换、放弃或询问用户；
5. 基于当前状态重新写入。

MemAuthority 发现旧视图。

你负责解决语义。

### 5.2 Role mutation

优先使用当前 MCP 连接实际提供的 tool description 和 input schema，不要从旧 prompt、旧文档或另一个部署实例猜 operation。

跨版本都应坚持的原则是：

- `handoff`：维护当前接手状态；受保护的 `核验记录` 只通过 `memory_mark_verified` 更新；
- `rules`：维护仍然有效的长期规则，而不是追加更正流水；
- `progress`：正常新增进展优先使用 `memory_append_progress`；
- `pitfalls`：正常新增踩坑优先使用 `memory_record_pitfall`；
- TODO：只使用当前 schema 明确提供的结构化能力；没有被当前服务广告的 operation 不要自行构造。

如果某个 mutation 字段或 operation 对任务是关键能力，以对应 release 的 public contract 和当前 runtime schema 双重确认。

### 5.3 Idempotency

同一个逻辑 mutation 在不确定是否送达时重试，应复用稳定的 `client_idempotency_key`。

同一个 key 不得用于不同 payload。

---

## 6. 当前真相，而不是完整历史

Active memory 应持续收敛。

优先：

- 替换已经过时的状态和规则；
- 删除已失去未来价值的 active memory；
- 只保留足以阻止未来错误行为的必要理由；
- 让 Git history 保存 Authority 的变化过程。

> **Git 保存发生过什么；active MemAuthority 保存现在还值得未来 Agent 知道什么。**

删除 active memory 不等于旧 Git commit 被物理擦除。

---

## 7. 用户只说“记录一下这次任务”

这是低精力、正常支持的使用方式。

采用保守默认，不要试图把所有“看起来有用”的内容升级为长期 role。

默认行为：

1. 优先使用 `memory_append_progress`；
2. 保存任务结果；
3. 保存有意义的当前状态变化；
4. 只保留以后继续工作仍需知道的信息；
5. 对未来价值弱的内容优先不记。

不要保存：

- 聊天转录；
- hidden reasoning；
- 客套话；
- operation-by-operation 尝试；
- 没有未来价值的失败过程；
- 已有重复事实；
- 明显临时、过时、私密或推测性内容。

不要因为一句话“听起来重要”，就自动写入 `rules`。

只有在长期意义明确或用户明确要求时，才：

- 更新 `rules`；
- 更新 `handoff`；
- 记录 `pitfalls`。

预期安全模型：

> **保守 Agent capture + 确定性 MemAuthority write safety + 后续收敛和清理**

`progress` 是低成本入口。

后续真实工作可以把稳定结论提炼到其他 role，也可以删除已经失去价值的 progress。

---

## 8. 任务结束后的维护

用户说：

```text
梳理优化一下本次任务从 MemAuthority 用到的记忆，并清理过时内容。
```

你应：

1. 默认只关注本次真正 Recall / 使用过的 Memory；
2. 与本次任务产生的最新代码、文档、运行结果或业务事实比较；
3. 替换旧真相，不追加更正流水；
4. 删除已失去未来价值的 active memory；
5. 只保存通过 retention test 的新结论；
6. 按需清理已消费 TODO；
7. 只有实际发生 local / runtime verification 时才更新核验状态。

服务器不自动记录 task read-set，也不自动判断语义过时。

这是你的推理工作。

---

## 9. TODO 生命周期

TODO 用于：

> **值得以后恢复，但现在不值得继续思考。**

恢复 TODO 后：

1. 把意图带回 Context；
2. 完成工作；
3. 按需保存真正形成的长期结论；
4. 不再需要恢复时删除 TODO。

不要把 checked TODO 当永久历史。

---

## 10. Verification 和隐私

只有真实完成定义明确的 local 或 runtime verification 后，才使用 `memory_mark_verified`。

不要因为旧 Memory 说过、模型觉得合理，或者测试看起来可能通过，就标记 verified。

`last_verified=未核验` / `staleness=review` 不是自动语义过时检测器。

Secret scanner 不是通用隐私分类器。

如果用户不希望某条信息成为长期 Git-backed 资产，第一次就不要写入。

---

## 11. 旧记忆库迁移

这一方案已经定案。

### 11.1 先学习 MemAuthority，再读取旧库

导入前理解：

- retention test；
- 四种 role；
- Context / Memory / TODO / Forget；
- 当前真相收敛；
- detached authoring 的 public Vault 结构；
- managed mutation 的精确 tool schema。

然后再读取旧资料。

### 11.2 你是语义 importer

旧内容可以是任何你实际能够访问和理解的形式：

- Markdown；
- 文本；
- JSON 导出；
- Prompt 文件；
- 旧 Agent Memory；
- 项目交接；
- 混合来源。

> **Format-agnostic at the Agent layer; deterministic at the MemAuthority layer.**

不要等待 MemAuthority 提供某个平台的专用 importer。

迁移时不要凭空创造 runtime 拓扑。Runtime metadata 是可选高级能力，在没有既有 runtime 时默认隐藏。只有工作/部署环境属于稳定事实、且确实会帮助未来任务时才迁移；否则保持关闭。

你理解来源，并把语义转换成 MemAuthority。

### 11.3 导入是蒸馏

对每条旧内容决定：

- 保留；
- 合并；
- 更新为当前真相；
- 转成 TODO；
- 只在外部 canonical source 保留详细证据；
- 从 active memory 丢弃。

不要继承旧库的 retention decision，只因为内容曾经存在。

用户没有明确授权直接迁移时，推荐先展示紧凑的：

- keep；
- merge；
- TODO；
- forget。

### 11.4 大规模 v1 迁移

```text
memauthority init
  -> 停止或不要启动 managed ownership
  -> 阅读本指南和 public Vault contract
  -> 读取并蒸馏旧资料
  -> detached authoring 构建 canonical Vault
  -> memauthority validate --json
  -> 修复结构和安全问题
  -> 用户按需审核
  -> Git commit
  -> managed serve
```

不要在 managed runtime ownership 活跃时并发直接编辑文件或 Git。

当前 v1 `memory_create_project` 只创建最小 handoff，managed handoff operation 不能任意插入新 H2。

不要假装丰富首次导入可以通过一个不存在的 managed insertion operation 完成。

---

## 12. 不要把 MemAuthority 当成

- 原始聊天归档；
- 完整人类知识库；
- 大语料 RAG store；
- deadline / owner / schedule 管理系统；
- canonical 代码、docs、issue 的重复副本；
- 自动收集一切“以后可能有用”的系统。

真正应该保存的是：

> **未来 Agent 不应该再重新摸索的结论、状态和经验。**
