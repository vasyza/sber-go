# Audited Python compatibility material

All data is synthetic, copied from the explicitly audited `fork/tests` source
set or deterministically derived by offline stdlib-only reference functions on
those copied inputs. Each vector records file/line/SHA-256 provenance. No real
profiles, cookies, HARs, credentials, deviceprints, bank data or network responses
were read or manufactured.

- `srp.json`: existing SHA-512 frontend known answer, including proof padding.
- `rsa.json`: **mock payload + invalid-key cases only**, not a genuine OAEP
  cryptographic known-answer vector. No audited real OAEP fixture was available.
- `frontend-config.json`: PIN/primary literal configs and existing negative cases.
- `session.json`: audited schema1/2/3/4 synthetic state and cookie metadata.
- `history.json`: amount vs balance-after, malformed optional/core amounts and
  sequential N+1 sentinel pagination.
- `resources.json`: all account kinds, details, card limits/credit and PFM models.
- `cookie-metadata.json`: existing SameSite parameter tables and Netscape input.
- `mutations.json`: the existing five-request own-resource synthetic workflow.
- `parameter-tables.json`: every audited parameterized test axis.
- `python-test-contracts.json`: all 199 test definitions, expected assertions,
  raises and fake setup/helpers; 183 runtime tests +16 maintenance tests.
- `fixture-provenance.json`: digests/counts and declared coverage limitations.
- `parity.json`: frozen upstream API and test acceptance inventory.
- `source-manifest.json`: audited public source paths and hashes.
- `verify_inventory.py`: development-only AST/manifest/provenance checker.

The frozen inventory keeps its original `pending` acceptance statuses.
Those statuses describe the reference audit, not current native test results.
See [the current verification record](../../docs/STATUS.md). The Python verification aid is
never a runtime dependency of the native Go SDK/CLI/MCP, and the recorded Python
source strings are evidence only, not an adapter to run the SDK.

## Offline inventory check

Run from the Go repository:

```sh
python3 testdata/compat/verify_inventory.py --source-root /path/to/audited/fork
```

The checker reads only manifest-listed public source/tests and these local
catalogs. It makes no network/auth/bank call and writes no files. Supply the audited public Python source checkout explicitly with `--source-root`.
The checker resolves source files relative to that checkout and retains the manifest hashes.
It does not need the original absolute workspace path.
Catalogs use relative source paths and omit host-specific checkout locations.
Source hashes, line ranges, and synthetic fixture contents retain their audited values.
The Python checkout is a development reference, not a Go runtime dependency.

## Attribution

MIT License

Copyright (c) 2026 sber-mcp contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
