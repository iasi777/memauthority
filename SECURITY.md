# Security

[中文](SECURITY_ZH.md)

## Supported line

The currently supported release is `v1.3.2` on the v1 compatibility line. Security fixes that preserve the frozen public surface use patch releases; incompatible security changes require a new major version.

## Deployment safety

When deploying, follow the current frozen [transport/auth](docs/contract/v1.3.2/transport-auth.md), [managed runtime](docs/contract/v1.3.2/managed-runtime.md), and [structured error](docs/contract/v1.3.2/structured-errors.md) contracts. Do not bypass loopback/authentication, clean-snapshot, fencing/CAS, state-containment, or fail-closed checks just to make a deployment start working.

## Reporting

Do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability reporting flow:

- [Privately report a security vulnerability](https://github.com/iasi777/memauthority/security/advisories/new)

Include the affected version, impact, reproduction details, and any suggested mitigation. Do not include real credentials, private Vault contents, or production secrets in the report.
