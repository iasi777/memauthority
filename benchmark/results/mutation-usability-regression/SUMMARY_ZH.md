# MemAuthority 跨会话交接：十轮重复性验证

验证日期：2026-09-08。Benchmark runner：`2.0.0-dev`。

MemAuthority 在 `handoff-pending-01` 专项验证中完成了 10 轮重复运行，覆盖三种 Agent Stack，共 30 次完整 case 运行、90 个独立 session。所有运行均通过自动验收和 benchmark 所有者的人工语义确认。这为当前构建在该场景下保存、恢复和完成跨会话待处理工作提供了重复性证据。

## 验证任务

每次运行从独立初始状态开始，跨三个全新 session 维护一个 Python retry module：

1. Session 1：修复仅对 `RetryableError` 重试的逻辑，默认最多 3 次。
2. Session 2：记录已确认的未来默认值 5，保持代码默认值 3，将改动保存为 pending work。
3. Session 3：从 MemAuthority 恢复 pending=5，完成实现，保留异常约束，更新记忆并消除过期的活动待办。

一轮指三个 Agent Stack 各完成一次三会话 case。评价单位包括 runner、model 和冻结配置；这些结果不构成纯模型排名。

## 结果

| Agent Stack | 完整 case 通过 | Session 完成 | 人工确认 | Mutation 错误 / 尝试 |
|---|---:|---:|---:|---:|
| GPT-5.6 Sol / Codex CLI | 10/10 | 30/30 | 10/10 | 0/136 |
| GPT-5.6 Luna / Codex CLI | 10/10 | 30/30 | 10/10 | 0/138 |
| Gemini 3.8 Flash / Pi Agent | 10/10 | 30/30 | 10/10 | 2/152 |
| 合计 | 30/30 | 90/90 | 30/30 | 2/426 |

90 个 session 的自动验收和语义判定均通过。人工确认由 benchmark 所有者和审计者（Claude Opus 5）共同完成，审计报告见 [`AUDIT_REPORT.md`](AUDIT_REPORT.md)。Mutation 错误按 MCP 日志中的 JSON-RPC error 或 isError=true 结果计数；2 次错误均为 Gemini agent 的 schema validation 失败（传入了非法字段 "witness"），agent 在重试后成功，未阻止最终完成。两个 GPT-5.6 stack 在所有 mutation 调用中零错误。

本 campaign 使用每 session 360 秒有效执行预算。Runner 仅扣除可识别的 provider 重试等待；外部中断后的重试以 stage 为单位，恢复 stage 开始前的完整快照并创建新 session。此次 30 次运行记录的外部等待均为 0，排除的 provider 尝试数也均为 0。预算和恢复规则见 [SPEC](../../SPEC.md)。

## 产品改进与证据边界

早期回归在 handoff mutation 上复现了猜测 section 名称、非法操作及误写 protected section 的问题。修复使读取结果暴露准确标题、允许的操作和专用工具路由，并增加普通 section 的显式插入、删除能力；验证记录继续通过专用工具维护。可选待办 section 可以缺省或为空，Agent 仍负责理解和维护 pending work。

这延续了 MemAuthority 提供持久化状态与受控写入、Agent 负责语义判断的分工。详细改动及历史对照见 [回归报告](REPORT.md)。历史对照与本次 campaign 使用不同 runner 预算规则，不能直接合并为严格控制变量的性能比较。

本结果支持该固定任务、三个 Agent Stack 和指定构建下的跨会话交接重复性。它不覆盖整个 benchmark suite，也不能推导任意任务、长期部署或所有模型的可靠性；没有无记忆对照组，不能据此量化相对无记忆方案的收益。Grok 的早期修复验证见历史报告，不属于本次三种 Stack 的十轮 campaign。

两份 Gemini 诊断 replay 已通过人工确认，分别提供从 Stage 2 快照恢复 Stage 3，以及从 Stage 1 快照继续 Stage 2/3 的证据。它们保留 `diagnostic_only=true`，单独呈现；早期空模板、runner 失败、不完整及 provider 无效记录均不计入当前统计。

## 构建与审计

30 次运行的产品提交、源码摘要和二进制摘要一致：

- 产品提交：`94eb36aa77ccdce3e057817d9d5133a62731912b`
- 源码 SHA-256：`cbedde22b24baee1dba235ced23d89c1e8f88d2624e9bbf34807c2cd915f84de`
- 二进制 SHA-256：`6981dfdc57c37a55da5c8fe6b0277f4739fe52780a3c1a19e887c428432c555e`
- 二进制版本字符串：`memauthority 1.3.2`。该构建包含后续 mutation usability 修复，应以提交和摘要识别，不应将本结果归于未经修复的 1.3.2 发行版。

[语义审计汇总](semantic-review-audit.json) 包含本 campaign 的 30 条记录。筛选口径为 `runs/*/manifest.json` 中 `benchmark_version == "2.0.0-dev"` 且 `case_id == "handoff-pending-01"`。每次运行的 `manifest.json`、`report.json`、`semantic-review.json`、冻结输入和分阶段 MCP 日志、项目及 vault 快照构成证据链。Mutation 计数直接来自每个 run 的分阶段 MCP 日志和记录字段。

发布时应一并提供这 30 条运行的证据 artifact，并维持相对路径或提供明确的 artifact 映射；仅上传本文不能替代原始证据。打包入口和运行方法见 [Benchmark README](../../README.md)。
