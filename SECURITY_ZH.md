# 安全政策

[English](SECURITY.md)

## 支持版本

当前版本为 v1 兼容线上的 `v1.3.1`。在冻结公共接口范围内完成的安全修复使用 patch 版本；不兼容的安全变更必须遵循冻结的 major 版本流程。

## 部署安全

请遵守当前冻结的 [Transport / Auth](docs/contract/v1.3.1/transport-auth.md)、[Managed Runtime](docs/contract/v1.3.1/managed-runtime.md) 和[结构化错误](docs/contract/v1.3.1/structured-errors.md)契约。不得为了绕过部署问题而削弱 loopback、认证、干净快照、fencing / CAS、状态目录隔离或 fail-closed 检查。

## 漏洞报告

如怀疑存在安全漏洞，请勿创建公开 Issue。请使用 GitHub 的私密漏洞报告入口：

- [私密报告安全漏洞](https://github.com/iasi777/memauthority/security/advisories/new)

报告中应包含受影响版本、影响范围、复现方法和建议缓解措施。请勿在报告中附带真实凭据、私有 Vault 内容或生产 Secret。
