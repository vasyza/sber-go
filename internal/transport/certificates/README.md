# Bundled bank CA

`russian_trusted_root_ca.pem` is the public **Russian Trusted Root CA** certificate. `roots.go` embeds it at compile time, so native SDK, CLI and MCP requests need no external bank certificate file or runtime certificate download. Default trust adds this root to the first available canonical system PEM bundle, or uses the embedded root alone when no system bundle is available. TLS chain, validity and hostname checks remain enabled. No operating-system trust store is changed.

Verified on 2026-10-07 against the [official certificate download](https://gu-st.ru/content/lending/russian_trusted_root_ca_pem.crt), linked from the [Gosuslugi certificate page](https://www.gosuslugi.ru/crt). The DER bytes also match the public root in the owner-provided PEM. [Sber's certificate guide](https://developers.sber.ru/help/certificates/how-to) explains the distinction between root and issuing certificates.

| Field | Value |
| --- | --- |
| Subject / issuer common name | `Russian Trusted Root CA` |
| Organization | `The Ministry of Digital Development and Communications` |
| Valid from (UTC) | `2022-03-01 21:04:15` |
| Valid until (UTC) | `2032-02-27 21:04:15` |
| DER SHA-256 | `d26d2d0231b7c39f92cc738512ba54103519e4405d68b5bd703e9788ca8ecf31` |

Only the self-signed root is bundled. The server supplies intermediate certificates for chain construction; an issuing CA is not added as a separate trust anchor.

For CA rotation, obtain the replacement from its official source, verify its provenance, fingerprint, CA constraints and self-signature, then update the PEM, pinned fingerprint test and this record together. Rebuild and run the local TLS suite plus the separately owner-authorized live check. Certificates are never updated by an implicit network request.

An explicitly selected `TransportOptions.CABundle` or `--ca-bundle PATH` replaces all default trust for that client. Invalid explicit files fail closed; they do not fall back to the embedded root. This override supports a deliberately selected replacement bundle without requiring a new build.
