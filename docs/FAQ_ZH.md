# V-Memory 常见问题

[English](FAQ.md)

这份 FAQ 只回答使用时最容易反复遇到的问题。

精确 API、schema、安全行为和版本兼容规则，以版本化公共契约 [`contract/v1.1/`](contract/v1.1/) 为准。

---

## V-Memory 会自动决定什么值得记吗？

不会。

V-Memory 负责的是可靠状态，不负责替你决定人生里什么重要，也不负责替 Agent 完成语义判断。

通常由 Agent 根据当前任务提出候选，你决定保留、修改还是放弃。

如果你只说“记录一下这次任务”，Agent 可以采用保守默认：优先记录仍有未来价值的 `progress`，而不是把所有内容都升级成长期规则。

---

## 为什么不直接继续用 MEMORY.md？

如果 `MEMORY.md + Agent` 已经够用，就继续用。

V-Memory 适合的是另一种需求：你希望记忆可以长期收敛，并且修改、并发写入、重试和中断都有明确边界。

它不是为了让所有用户都必须迁移。

---

## V-Memory 会不会把每次聊天都保存下来？

不会，也不应该。

完整聊天、客套话、临时尝试、隐藏推理和没有未来价值的失败过程，都不应该因为“以后也许有用”就自动进入长期 Memory。

判断标准很简单：

> 未来 Agent 知道这件事，是否会改变行为、恢复必要状态、减少明显的重新推理，或者避免重复犯错？

如果不会，大概率不需要保存。

---

## “记录一下这次任务”会不会越记越乱？

风险不能靠一句“自动整理”彻底消失。

V-Memory 的做法是把责任拆开：

- Agent 保守选择内容；
- V-Memory 保证 revision、CAS、幂等、验证、secret scanning 和 transaction safety；
- 后续真实任务继续更新或删除已经失去价值的 active memory。

所以 `progress` 是低成本入口，不是永久档案。

---

## 四种 role 应该怎么选？

只需要记住它们的意图：

- `handoff`：现在接手这个项目必须知道什么；
- `rules`：以后仍然要遵守什么；
- `progress`：哪些阶段进展以后仍有价值；
- `pitfalls`：哪些失败模式以后仍可能再次踩到。

不要为了“结构完整”强行填满四种 role。

---

## TODO 是任务管理器吗？

不是。

V-Memory TODO 表示：

> 以后值得恢复，但现在不值得继续占用注意力的意图。

它不负责 deadline、owner、排期、依赖和团队协作。

TODO 完成后通常应该删除；真正形成的长期结论，再单独进入 Memory。

---

## 为什么不在任务开始时一次性加载所有记忆？

因为更多 Context 不等于更好的判断。

正确做法不是固定执行一套 Recall 仪式，而是按当前信息状态选择最小动作：

- 当前 Context 已经足够 → 不调用 V-Memory；
- 项目不确定 → 先确认项目；
- 已知 URI / section → 直接精确读；
- 已知项目但不知道位置 → 再搜索；
- 需要恢复项目整体状态 → `handoff` 通常最值得先读；
- 只展开真正相关的部分。

> Search results are coordinates, not context.

搜索结果是定位坐标，不是自动塞进 Prompt 的内容。

---

## 多个 Agent 同时使用会不会互相覆盖？

V-Memory 的核心模型是：

> Multi-Agent, single Authority.

多个 Agent 可以共享一份 Memory，但写入必须基于当前 revision。

如果另一个 Agent 已经更新了内容，拿旧 revision 的写入应该发生 conflict，而不是静默覆盖。

V-Memory 负责暴露分歧；Agent 负责重新读取后解决语义。

---

## Validation 通过是不是说明记忆内容一定正确？

不是。

Validation 检查的是结构、安全和确定性约束，例如 Vault 格式、路径、日期同步、UTF-8、source limit 和高置信度 secret。

它不会判断：

- 这条记忆是否真的重要；
- 结论是否聪明；
- 内容是否已经语义过时；
- 用户是否希望长期保存它。

所以 validation 通过以后，active memory 仍然可能需要人工或 Agent 审核。

---

## 我已经有几千行 MEMORY.md，必须手工改格式吗？

不用。

推荐让 Agent 做语义迁移：

1. 先理解 V-Memory 的 retention 和 role；
2. 再读取旧库；
3. 去重、合并、更新和淘汰；
4. 只把仍值得长期保存的内容写入新 Vault；
5. validate、review、commit，再进入 managed 使用。

导入是蒸馏，不是复制。

---

## 为什么不做一个通用 importer？

因为真正困难的不是 Markdown、JSON 或某个平台的文件格式，而是：哪些内容仍然有效、哪些冲突、哪些只是历史、哪些不应长期保存。

这些属于语义判断，更适合由能够理解来源和用户意图的 Agent 完成。

---

## Managed service 运行时还能直接编辑 Vault 文件吗？

不要并发这样做。

Detached authoring 允许直接编辑 Git / Markdown / YAML；managed ownership 启动以后，Authority mutation 应走受控 MCP path。

需要大规模直接重构时，先停止 managed ownership，完成 detached authoring、validate、review 和 commit，再重新启动 managed service。

---

## 删除一条 Memory 后，Git history 里也消失了吗？

没有。

Active V-Memory 保存当前仍值得未来 Agent 知道的内容；Git 保存 Authority 的变化历史。

所以删除 active memory 不等于历史 commit 被物理擦除。

如果某条信息不应该成为长期 Git-backed 资产，最可靠的方式是第一次就不要写入。

真正需要历史清除时，应按独立的数据清除 / Git history rewrite 流程处理。

---

## Secret scanner 能识别所有隐私吗？

不能。

V-Memory 会拦截高置信度 secret 形态，但它不是通用隐私分类器。

家庭地址、私人经历、商业机密、未公开计划等内容是否应该长期保存，仍然需要用户和 Agent 判断。

---

## V-Memory 是 RAG、知识库或者聊天归档吗？

都不是。

- 大量资料检索更适合 RAG / 搜索系统；
- 完整项目知识更适合 README、docs、Wiki、ADR；
- 原始对话应该留在聊天或日志系统；
- deadline / owner / schedule 应交给任务管理工具。

V-Memory 保存的是：

> 未来 Agent 不应该再重新摸索的东西。
