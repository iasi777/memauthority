# MemAuthority Benchmark

MemAuthority maintains a public, auditable benchmark suite that evaluates cross-session memory reliability under real agent workloads.

---

## Latest Results: Mutation Usability Regression (2026-09-08)

### Campaign: handoff-pending-01

Cross-session project handoff with pending work tracking across 3 independent sessions.

### Results Summary

| Metric | Result |
|---|---:|
| **Total mutation operations** | **426** |
| **Mutation errors** | **2 (0.5%)** |
| Runs completed | 30/30 (100%) |
| Sessions completed | 90/90 (100%) |
| Human semantic review | 30/30 confirmed |
| Audit confidence | HIGH |

### By Agent Stack

Each stack completed **10 rounds** (10 runs × 3 sessions = 30 sessions per stack):

| Agent Stack | Runs | Sessions | Mutation Attempts | Mutation Errors |
|---|---:|---:|---:|---:|
| Codex CLI + GPT-5.6 Sol | 10/10 | 30/30 | 136 | **0 (0.0%)** |
| Codex CLI + GPT-5.6 Luna | 10/10 | 30/30 | 138 | **0 (0.0%)** |
| Pi Agent + Gemini 3.8 Flash | 10/10 | 30/30 | 152 | **2 (1.3%)** |

**Total across all stacks**: 30 runs, 90 sessions, **426 mutation operations, 2 errors (0.5%)**

### Key Findings

✓ **Cross-session handoff works reliably**  
All 30 runs successfully preserved project constraints, tracked pending work distinctly from current state, and recovered pending work in the final session.

✓ **High mutation API usability**  
The corrected error rate of **0.5% (2/426 operations)** demonstrates strong API usability. Both GPT-5.6 stacks achieved zero mutation errors across 274 combined operations.

✓ **Error recovery demonstrated**  
The 2 errors (both in the same run, same tool, same validation issue) were recovered successfully. Agent retried and completed all acceptance criteria.

✓ **Independent audit certification**  
Comprehensive audit by Claude Opus 5 with 100% artifact coverage, explicit methodology, and high confidence determination.

### Audit Correction Note

**Original report**: 18/426 mutation errors (4.2%)  
**Audited actual**: 2/426 mutation errors (0.5%)

The original count could not be reproduced from MCP logs. Comprehensive audit with explicit criteria (JSON-RPC error OR isError=true in tool result) found only 2 errors. Discrepancy analysis and complete methodology documented in audit report.

---

## What Was Tested

**Scenario**: Cross-session project handoff with pending work tracking

Each run executed 3 fully independent Agent sessions (no shared dialogue or context):

1. **Stage 1 (Establish)**: Fix Python retry module, document constraints in memory
2. **Stage 2 (Maintain)**: Record pending future change without implementing it
3. **Stage 3 (Use)**: Recover pending work from memory and implement correctly

**Success criteria**: Agent must maintain information across independent sessions using only MemAuthority's durable project memory.

---

## Documentation

### Audit Documents
- [**AUDIT_SUMMARY.md**](../benchmark/results/mutation-usability-regression/AUDIT_SUMMARY.md) — Executive overview and recommendation
- [**AUDIT_REPORT.md**](../benchmark/results/mutation-usability-regression/AUDIT_REPORT.md) — Comprehensive technical audit with methodology
- [**SEMANTIC_REVIEW_CONFIRMATION.md**](../benchmark/results/mutation-usability-regression/SEMANTIC_REVIEW_CONFIRMATION.md) — Semantic correctness certification
- [**AUDIT_COMPLETION_CERTIFICATE.md**](../benchmark/results/mutation-usability-regression/AUDIT_COMPLETION_CERTIFICATE.md) — Official audit certification

### Campaign Results
- [**README.md**](../benchmark/results/mutation-usability-regression/README.md) — Result documentation guide
- [**SUMMARY_ZH.md**](../benchmark/results/mutation-usability-regression/SUMMARY_ZH.md) — Complete Chinese summary
- [**REPORT.md**](../benchmark/results/mutation-usability-regression/REPORT.md) — Brief English summary

### Data Files
- [**mutation_error_audit.json**](../benchmark/results/mutation-usability-regression/mutation_error_audit.json) — Machine-readable error analysis
- [**semantic-review-audit.json**](../benchmark/results/mutation-usability-regression/semantic-review-audit.json) — Complete review data with metadata

### Evidence
Complete evidence artifacts (MCP logs, project snapshots, vault snapshots, frozen inputs) for all 30 runs are available as GitHub Release Assets.

**Download**: [benchmark-v1-mutation-usability-regression-runs-20260908.tar.gz](https://github.com/iasi777/memauthority/releases)

---

## Benchmark Specifications

- [**SPEC.md**](../benchmark/SPEC.md) — Benchmark specification 2.0.0-dev
- [**README.md**](../benchmark/README.md) — Benchmark overview and usage

---

## Audit Certification

**Auditor**: Claude Code (Opus 5) / Claude (Anthropic)  
**Model**: claude-opus-5[1M]  
**Audit Date**: 2026-09-08  
**Coverage**: 100% (30 runs, 90 sessions, 426 operations)  
**Methodology**: Comprehensive MCP log analysis, direct artifact inspection, explicit criteria  
**Confidence**: HIGH  
**Determination**: PASS — All runs semantically correct, evidence quality excellent  
**Recommendation**: APPROVED FOR PUBLICATION  

---

## Product Information

**Product**: MemAuthority  
**Version tested**: Post-1.3.2 with mutation usability fixes  
**Commit**: 94eb36aa77ccdce3e057817d9d5133a62731912b  
**Binary SHA-256**: 6981dfdc57c37a55da5c8fe6b0277f4739fe52780a3c1a19e887c428432c555e  

---

## Citation

```
MemAuthority Benchmark v1 — Mutation Usability Regression Campaign
Case: handoff-pending-01, 10 rounds, 3 Agent Stacks
Executed: 2026-09-08
Audited: 2026-09-08 by Claude Code (Opus 5)
Result: 30/30 PASS, 426 mutation operations, 2 errors (0.5%)
Product: MemAuthority commit 94eb36aa (post-1.3.2 with usability fixes)
```

---

**Back to**: [Main Repository](../)
