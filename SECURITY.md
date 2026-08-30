# Security

[中文](SECURITY_ZH.md)

## Supported line

The current release is `v1.2.0` on the v1 compatibility line. Security fixes that preserve the frozen surface are patch-version changes under the frozen SemVer policy; incompatible security changes require the frozen major-version path.

## Deployment safety

Follow the current frozen [transport/auth](docs/contract/v1.3/transport-auth.md), [managed runtime](docs/contract/v1.3/managed-runtime.md), and [structured error](docs/contract/v1.3/structured-errors.md) contracts. Do not weaken loopback/authentication, clean-snapshot, fencing/CAS, state-containment, or fail-closed checks to work around deployment issues.

## Reporting

Do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability reporting flow:

- [Privately report a security vulnerability](https://github.com/iasi777/memauthority/security/advisories/new)

Include the affected version, impact, reproduction details, and any suggested mitigation. Do not include real credentials, private Vault contents, or production secrets in the report.
