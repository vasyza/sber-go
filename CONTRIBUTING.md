# Contributing to sber

The CLI and public Go package are named `sber`.
The module path is `github.com/vasyza/sber-sdk`.

## Local development

1. Install Go 1.27.1 or later and Git.
2. Clone the repository:

   ```sh
   git clone https://github.com/vasyza/sber-sdk.git
   cd sber-sdk
   ```

3. Run the complete checks:

   ```sh
   make check
   ```

The checks include formatting, golangci-lint, gopls diagnostics, vet, race tests, builds, and module verification.
The Makefile installs pinned analysis tools in the ignored `bin/` directory.
Default tests use synthetic data and localhost.
They need no bank account, credentials, browser, or `.env` file.

See [the development guide](docs/DEVELOPMENT.md) for separate checks and documentation rules.

## Changes and pull requests

Add a failing regression test for a behavior change, then implement the change.
Keep package responsibilities and public contracts explicit.
Preserve TLS verification, secret redaction, private file permissions, and read-only defaults.
Do not make tests contact the bank without a separate explicit authorization.
Use Conventional Commits for commit subjects.
Include the behavior change and verification results in the pull request description.

Keep upstream attribution in `LICENSE`, `NOTICE`, and `third_party/`.
Contributions to this repository use the project MIT license.

## Issue reports

Include the command name, diagnostic message, operating system, and Go version.
Use synthetic examples to show the problem.
Do not attach real credentials, profiles, cookies, HAR files, QR challenges, or account data.
Follow [SECURITY.md](SECURITY.md) for vulnerability reports.

## Release checks

Run `make check` before publication.
For a dependency vulnerability check, run:

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

Review tracked files and Git history for secrets before changing repository visibility.
Record verification limits in [STATUS.md](docs/STATUS.md).
