# Security policy

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/vasyza/sber-go/security/advisories/new).
Select **Report a vulnerability** in the repository security tab.
Do not put an unpatched vulnerability or private account data in a public issue.

Include the affected commit or release, impact, and a reproduction with synthetic data.
Do not include bank passwords, PINs, SMS codes, cookies, profiles, HAR files, or financial data.

## Supported versions

Security fixes target the current `main` branch and the latest release.
Update the CLI with the command in [README.md](README.md#update-the-cli).

## Security boundaries

TLS verifies the certificate chain and hostname.
The bundled CA is a public certificate and contains no private key.
Profiles contain session credentials and must remain private.
Diagnostic output redacts secrets; explicit data exports can contain financial information.
The default policy permits reads; financial changes require explicit execution and confirmation.

Local tests and static analysis do not prove that a bank session remains valid.
See [STATUS.md](docs/STATUS.md) for current verification limits.
