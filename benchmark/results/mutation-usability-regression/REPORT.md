# Mutation usability regression

当前发布目录只保留修复后 `2.0.0-dev` 的 canonical campaign：

- `handoff-pending-01`，10 轮；
- 3 个 Agent Stack；
- 30 次完整 case run、90 个独立 session；
- 30/30 自动验收通过，30/30 人工语义确认通过；
- mutation 错误 2/426 (0.5%)，全部为 Gemini agent schema validation 失败后重试成功；GPT-5.6 stacks 零 mutation 错误。详见审计报告。

完整中文总结见 [`SUMMARY_ZH.md`](SUMMARY_ZH.md)。逐 run 原始证据位于 [`../../runs`](../../runs)，人工确认与证据路径审计见 [`semantic-review-audit.json`](semantic-review-audit.json)，当前构建记录见 [`current-build-verification.json`](current-build-verification.json)。

早期 1.x runs、provider 中断、诊断 replay、临时 session/context 以及历史分析文件已从 active benchmark 目录移出，不参与当前统计。当前目录的统计口径只针对上述 30 条 canonical run 记录。
