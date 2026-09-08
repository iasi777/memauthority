
# MemAuthority Benchmark Specification 2.0.0-dev

## 1. Scope

Benchmark v1 评估 MemAuthority 的项目级长期记忆在真实 Agent 工程会话中的可靠性。它不要求 Agent 遵循固定的 `route -> search -> read` 检索流程；MemAuthority contract 是 capability guidance，Agent 可根据上下文选择最短路径。Benchmark 评价最终工程状态、记忆语义状态、隔离完整性与可观察证据，而不是偏好的工具调用顺序。

## 2. Canonical lifecycle

每个 canonical case 都使用三个全新独立 session：

1. **Establish**：建立事实、约束、决定、经验或待办。
2. **Maintain**：核验、更新、干扰、取消、完成或维护状态。
3. **Use**：不重述关键历史答案，要求 Agent 继续项目并正确行动或正确不行动。

三个 session 只连续共享 case workspace、隔离 MemAuthority Vault 和产品 state；不共享 Agent 对话、个人记忆、个人 MCP、插件、skills 或其他 benchmark case。

## 3. Capability families

| Family | Core | Extended | 目标 |
| --- | --- | --- | --- |
| Handoff | handoff-pending-01 | handoff-multistep-02 | 恢复明确未完成工作并继续实施 |
| Constraint retention | constraint-retention-03 | constraint-negative-04 | 后续修改不破坏长期正/负约束 |
| Supersession | supersession-basic-05 | supersession-cancelled-06 | 新裁决覆盖旧状态，不执行 stale decision |
| Gotcha / experience | gotcha-concrete-07 | gotcha-transfer-08 | 记录并迁移可复用工程经验 |
| Isolation / relevance | isolation-project-09 | isolation-alias-10 | 多项目/近似标识下不串记忆 |
| Unknown / stale resistance | unknown-none-11 | stale-completed-12 | 无待办时不编造，完成/取消后不重做 |

## 4. PASS model

每个 stage 有三类证据：

- **engineering**：隐藏 acceptance 在 Agent 进程外运行；
- **protocol/integrity**：session 完成、Vault 可验证、隔离成立、实际 MemAuthority schema 可观察、bridge 正常关闭等；
- **semantic review**：按 case rubric 审阅工程快照、Vault 快照与工具轨迹，确认语义状态正确。

Case PASS 需要三个 stage 都通过自动验收并完成 semantic review。Semantic review 是官方判定的一部分，但**不评价固定 retrieval path**。v1 的公开结果要求人工最终确认 semantic review；LLM 可以辅助起草。

## 5. Product gate vs Agent results

- **Product Gate**：MemAuthority 自身确定性测试；release blocker。
- **Reference Suite**：reference stack 的 12/12 canonical case 结果。
- **Cross-model Core Matrix**：5 个 Agent Stack × 6 个 Core cases，按实际结果公开，不把某个 Agent 行为失败自动等同为产品缺陷。

## 6. Failure taxonomy

- `invalid_run`: provider outage、限流、认证、外部进程异常等导致实验无效；不计 Agent fail。
- `runner_failure`: adapter、sandbox、trace collector 等 benchmark 基础设施失败；不计 Agent fail。
- `case_defect`: prompt、fixture 或 grader 有缺陷；不计 Agent fail。
- `product_defect`: MemAuthority 自身错误；计失败并触发产品调查。
- `agent_behavior_failure`: 工具可用但 Agent 决策/执行错误；计 Agent fail。
- `review_unresolved`: 证据不足或审阅未完成；暂不计。
- `unclassified`: 尚未定位；必须调查后重分类。

Case report 使用 `pass / fail / invalid / pending_review` 四种状态。`invalid_run`、`runner_failure`、`case_defect` 和尚未定位的 `unclassified` 汇总为 `invalid`，不计入 Agent failure rate；`product_defect` 与 `agent_behavior_failure` 才计为 `fail`。

执行预算规则（2.0 开发版）：profile 的 `timeout_seconds` 是有效执行预算，默认 360 秒。正常请求响应、思考、工程操作、工具调用和错误 mutation 重试均计时。只有客户端结构化错误事件明确给出的 provider 重试等待区间扣时；客户端报告恢复则提前结束扣时。长时间静默、普通 assistant 文本、工具结果中的报错字符串不构成停表证据。未支持的客户端事件格式保留原始证据供审阅，不能声称自动覆盖所有外部故障。

每次 session 保存 `elapsed_seconds`（进程执行墙钟）、`active_elapsed_seconds`（扣除等待）、`external_wait_seconds` 和不含原始错误正文的 `provider_events`。客户端错误已恢复且 session 正常完成时，不能仅因历史日志存在限流文字将其作废。预算耗尽且无尚未恢复的已确认 provider 故障时，沿用 `agent_behavior_failure / execution_timeout`；未开始有效工具活动的超时仍按 `invalid_run` 处理。

### 外部故障与阶段重试

- 已识别的限流、冷却、服务不可用和连接故障属于 transient；额度耗尽、订阅及认证错误属于 blocked。当前仅信任客户端结构化错误信封，不扫描整段对话关键词。
- 有明确 delay 的 transient 可以在同一 session 内等待恢复，每次尝试累计最多扣除 120 秒外部等待。累计上限也限制连续等待。错误未提供可测量的 delay 时，结束此次尝试，交给 stage 重试；不猜测停机时长。
- 同一调度轮每个 stage 最多 3 次尝试，transient 尝试之间等待 30 秒。这段调度等待不计执行预算。blocked 立即暂停，无需重复消耗请求。profile 可通过 `provider_policy` 覆盖上述正数配置，随 inputs 冻结；`max_stage_attempts` 必须为正整数。
- 重试最小单元为 stage。每次尝试从该 stage 起始前相同的 project、Vault、state 快照创建独立工作目录和全新 Agent session；不携带失败尝试产生的部分写入。每次重试重新获得完整有效执行预算，成功的前序 stages 保留。
- 外部中断尝试移动至 `excluded_attempts/<stage>/<attempt-id>`，保留已有日志、验收和快照；不计产品成功率分子或分母。preflight 认证阻断记录故障原因，没有 Agent 执行轨迹。报告主表仅使用最终保留的 stage，方法说明提供排除次数；原始错误 mutation 证据仍可审计。
- 达到重试次数上限或遇到 blocked 时，将 case 标记为 provider-paused，后续 stage 不执行。恢复后使用 `run.py --resume <run-dir>` 从暂停 stage 的起始快照重新调度。这里恢复的是 benchmark case，Agent 对话仍是全新 session。
- 非 provider 的阶段失败不自动重试，也不执行依赖它的后续阶段。尚未完成的 provider-paused case 为 invalid，不进入产品成功率分母。
- 恢复要求 frozen inputs 和产品二进制哈希一致、runner 与冻结版本一致。历史 v1 运行不套用新预算或自动改判。聚合仅选取当前 benchmark_version 的结果，禁止将旧 wall-clock 结果混入新预算样本。

## 7. Versioning

Benchmark 使用独立 SemVer。Patch 只允许无语义影响的 runner/report 修复；Minor 可增加 case、Agent Stack 或统计字段；Major 用于修改 canonical case 语义、PASS 规则、semantic rubric 或 lifecycle 定义。每个 case 还有独立 `case_version`。

## 8. Publication

Git 保存规范、代码、case、profile 和 summary results。完整 events、MCP request/response、session logs、project/Vault/state snapshots 作为 GitHub Release raw artifact 发布，并由仓库内 SHA-256 清单引用。历史结果永不覆盖。
