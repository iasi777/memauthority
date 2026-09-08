
# MemAuthority Benchmark 2.0 开发版

已完成的专项验证：[跨会话交接十轮总结](results/mutation-usability-regression/SUMMARY_ZH.md)（30 次完整运行、90 个独立 session，全部人工确认通过）。

当前开发版修改了外部 provider 故障下的执行预算与 stage 重试规则，版本为 `2.0.0-dev`。目录名仍为 `benchmark-v1`；active tree 只保留修复后的十轮 mutation usability regression campaign 及其证据。结果见 `results/mutation-usability-regression/REPORT.md`，早期测试材料已移出 active tree。

这是 MemAuthority 的公开、可审计跨会话 benchmark。它与历史目录 `benchmark/` 和诊断目录 `benchmark-minimal/` 完全分离：本目录拥有独立 cases、runner、profiles、runs 与 results，不读取或写入旧 benchmark 的运行结果。

## 目标

v1 核心目标是：

1. **产品/协议可靠性**：MemAuthority 在真实 Agent 使用中保持可用、隔离、可验证。
2. **跨会话记忆有效性**：Agent 能保存、维护、恢复并正确使用长期工程状态。
3. **发布/回归门禁**：产品升级后 canonical 能力不退化。

v1 不把“相对其他记忆方案的收益”作为发布门禁；外部 memory benchmark 可以作为独立研究证据接入。

## 官方评价单位

结果按 **Agent Stack = Runner + Model + frozen configuration** 报告，而不是把 Codex 与 Claude Code 下的数字伪装成纯模型排行榜。

首版 canonical 矩阵（历史发布配置）：

- Codex CLI + GPT-5.6 Sol
- Codex CLI + GPT-5.6 Luna
- Claude Code + Claude Opus 4.6
- Claude Code + Grok 4.6
- Claude Code + Gemini 3.8 Flash

## Suite

- Canonical suite 仍定义 6 个 **Core** cases 与 6 个 **Extended** cases；每个 canonical case 固定 3 个全新独立 session。
- `suite.json` 中的矩阵与 `published_repetitions` 描述 canonical suite 的发布配置，不代表后续专项 regression campaign 的轮数。
- 历史 1.x 的完整矩阵执行计划属于旧任务，已不作为当前待执行口径。
- 当前已完成的新任务是 `handoff-pending-01` mutation usability regression 的 **10 轮 campaign**：每轮各运行 Sol/Codex、Luna/Codex、Gemini/Pi 一个完整三会话 case，共 **30 case-runs / 90 sessions**。全部 30 个 report 为 `pass`，90/90 sessions 完成，且均已完成人工 semantic confirmation。
- 当前 active tree 不包含早期失败、provider 中断或 diagnostic replay；这些材料不参与十轮 canonical 统计。

查看 canonical suite 定义对应的执行计划（不是上述十轮 regression campaign）：

```bash
python3 suite.py --plan
```

运行一个 case：

```bash
python3 run.py --case handoff-pending-01 --profile codex-gpt-5.6-sol
```

生成或刷新单次 case report：

```bash
python3 run.py --report runs/<run-id>
```

外部服务恢复后，重试暂停的 stage：

```bash
python3 run.py --resume runs/<run-id>
```

360 秒预算扣除客户端明确报告的 provider 重试等待。默认每次尝试最多等待 120 秒，每轮最多尝试 3 次，间隔 30 秒；额度或认证问题立即暂停。每次重试恢复 stage 开始前的完整快照并创建新 session。排除的外部中断尝试保存在 `excluded_attempts`，报告只汇总数量，不计产品成功率。未知事件格式不自动扣时。详情见 SPEC 的外部故障规则。

聚合当前已有运行结果：

```bash
python3 suite.py --aggregate results/v1.0.0
```

把选定 raw runs 打包成 GitHub Release artifact（bundle 本身不要提交进 Git）：

```bash
python3 release.py --output /tmp/memauthority-benchmark-v1-raw.tar.gz runs/<run-a> runs/<run-b>
```

运行本地确定性测试（不会调用模型）：

```bash
python3 -m unittest discover -s tests -v
```

完整规范见 [SPEC.md](SPEC.md)，Runner 隔离契约见 [runners/CONTRACT.md](runners/CONTRACT.md)。
