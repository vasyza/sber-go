# Go migration boundaries

- Module: github.com/vasyza/sber-go; native Go runtime, no Python subprocess adapter.
- Source parity reference is the audited ex3lite/sber-mcp SDK and its synthetic fixtures. Preserve upstream MIT attribution.
- Implement test-first vertical slices (RED → minimal GREEN → refactor), never a placeholder counted as a port.
- Tests use localhost/synthetic data and must not reach a bank unless a separately bounded public-only probe is explicitly requested.
- Never import owner profiles, cookies, credentials, deviceprints, HARs, balances, account data or support IDs into source/tests/docs/commits.
- TLS/hostname checks and browser sandbox stay enabled. Ordinary public rendering is allowed; stealth, CAPTCHA solving, protection-cookie forgery and financial POST replay are not.
- Freeze pre-probe owner cookies; atomically adopt strictly validated rendered config/cookies/identity. Preserve SameSite names/values/scoping/deletion; closed state cannot resurrect transport.
- Defaults expose read-only operations; mutation parity is behind an explicit policy, never auto-enabled for monitoring. This does not narrow the bank's session privilege.
- Bank login secrets are local owner-operated hidden terminal input only, fail closed on echo-control failure. Proxy login values use the owner's requested explicit proxy argument format. Enrollment uses private lock + atomic no-replace publication.
- Partial/unavailable history means UNKNOWN, not unpaid; no tenant sends until owner validation.
- For production acceptance run go test -race ./..., go vet ./..., go build ./... and independent review. Describe live verification boundaries honestly.
- The owner explicitly requested this private WIP source handoff before completion. Publication of this snapshot is not production acceptance; historical failures are retained in Git history and regression tests. Current verification limits are in docs/STATUS.md. Autonomous work is paused; do not resume, repair, authenticate or contact tenants without a new owner request.
