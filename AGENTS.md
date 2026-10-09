# Development boundaries

- Module: github.com/vasyza/sber-sdk; native Go runtime, no Python subprocess adapter.
- Source parity reference is the audited ex3lite/sber-mcp SDK and its synthetic fixtures. Preserve upstream MIT attribution.
- Implement test-first vertical slices (RED → minimal GREEN → refactor), never a placeholder counted as a port.
- Tests use localhost/synthetic data and must not reach a bank unless a separately bounded public-only probe is explicitly requested.
- Never import owner profiles, cookies, credentials, deviceprints, HARs, balances, account data or support IDs into source/tests/docs/commits.
- TLS/hostname checks and browser sandbox stay enabled. Ordinary public rendering is allowed; stealth, CAPTCHA solving, protection-cookie forgery and financial POST replay are not.
- Freeze pre-probe owner cookies; atomically adopt strictly validated rendered config/cookies/identity. Preserve SameSite names/values/scoping/deletion; closed state cannot resurrect transport.
- Defaults expose read-only operations; mutation parity is behind an explicit policy, never auto-enabled for monitoring. This does not narrow the bank's session privilege.
- CLI authentication accepts SBER_LOGIN, SBER_PASSWORD, SBER_PINCODE, SBER_PHONE, and SBER_CARD_NUMBER from the process environment or an explicitly selected private --env-file. Environment values override file values; missing values use hidden terminal input. SMS codes and financial confirmations remain terminal-only, with no automatic challenge replay. Fail closed on echo-control failure. Proxy login values use the owner's requested explicit proxy argument format. Enrollment uses private lock + atomic no-replace publication.
- Partial/unavailable history means UNKNOWN; do not infer complete coverage from a partial response.
- Before publication run make check, including golangci-lint and gopls diagnostics. For production acceptance also require independent review. Describe live verification boundaries honestly.
- A source release is not production acceptance. Preserve historical failures in Git and regression tests. Keep current verification limits in docs/STATUS.md. Bank probes and financial actions require a separate explicit owner request.
