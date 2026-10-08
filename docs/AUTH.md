# Authentication and profiles

The CLI operator procedures, complete options, and session restoration are in [CLI.md](CLI.md) and [CLI-REFERENCE.md](CLI-REFERENCE.md). These Go API notes retain their engineering format. The latest real verification is in [STATUS.md](STATUS.md).

On Linux and macOS the native owner path is `./bin/sber login`. The CLI uses the current user configuration directory for its default profile. `--profile PATH` selects another file for one command. It acquires an enrollment lock before prompts, creates missing private parents and refuses an existing profile. Authentication reads `SBER_LOGIN`, `SBER_PASSWORD`, `SBER_PINCODE`, `SBER_PHONE`, and `SBER_CARD_NUMBER` from the process environment or an explicit private `--env-file PATH`. Environment values take precedence, including an empty value selecting terminal input. Missing values use hidden input; requested OTP is handled once through the terminal. Password/PIN/OTP are not profile fields. Echo-control failure stops input. CAPTCHA/WebAuthn are not automated CLI flows.

The credential file uses literal dotenv assignments, optional `export`, quotes and comments. It is bounded to 64 KiB and requires a symlink-free, owner-controlled `0600` single-link file in a private `0700` directory. File loading never modifies the process environment or persists credentials in the profile. `SBER_PINCODE` supplies both new-PIN enrollment and its confirmation when enrollment is required. Invalid fixed PIN input or a bank policy rejection stops instead of prompting repeatedly or sending the same PIN again. Interactive PIN input retains owner correction and confirmation.

macOS uses the terminal device path returned by the kernel, verified against the original input descriptor, and native exclusive rename for first publication. Linux uses its pinned procfs descriptor capabilities. Both restore terminal settings after success, failure or cancellation and preserve an existing profile. Profile paths must have literal, symlink-free components; use your home directory rather than macOS `/tmp` or `/var` aliases.

Native SDK, CLI and MCP requests use a verified Russian Trusted Root CA embedded in the build. Bank certificate trust needs no external file or runtime download, including when a system PEM bundle is unavailable. Default trust also retains a canonical system PEM bundle when present; `SSL_CERT_FILE` and `SSL_CERT_DIR` do not select alternative trust. TLS chain, validity and hostname verification remain enabled, and system trust stores are not changed. See [certificate provenance and rotation](../internal/transport/certificates/README.md).

An optional `--ca-bundle PATH` replaces default trust for that client. Embedded callers use `AuthOptions.TransportOptions.CABundle` or `ClientOptions.TransportOptions.CABundle`. An invalid explicit CA file fails before credential input and does not fall back to the embedded root. The CLI reports safe failure classifications without emitting raw bank messages, credentials or response bodies.

PIN enrollment announces the length from the validated live configuration. Invalid local digit input or a confirmation mismatch can be corrected within the same authentication process, without repeating login/OTP or submitting another PIN creation request. The final seamless navigation retains the originating `Process-Id`, matching the bank's public `r-97.0.0` login client contract.

Native authentication retains verified HTTP/1.1 connections across its steps. It rejects replay-enabling POST headers and never repeats a transmitted POST after an uncertain response, including the empty seamless-navigation POST. Business transports keep one request per connection. `TransportOptions.Timeout` bounds the complete request, including connection establishment, TLS verification and response reading; transport phases do not silently shorten this budget.

Direct TLS establishment advertises HTTP/1.1 through ALPN. A peer closure before TLS completion can cause at most three connection attempts within the original request budget. Certificate/protocol failures and cancellation stop establishment; recovery never replays a transmitted HTTP request. Synthetic TLS tests cover exact POST counts, the connection-attempt cap, trust failures, cancellation, and close during recovery. The final real primary-profile CLI and SDK E2E passed with this policy.

The CLI uses its saved proxy for native authentication.
`--proxy ADDRESS` replaces that setting; `--no-proxy` selects a direct connection.
HTTP, HTTPS, and SOCKS5 support optional proxy authentication.
Proxy errors stop requests without a direct connection.
TLS verification stays enabled, including for HTTPS proxies.
See the [proxy procedure](CLI.md#proxy-settings) and [SDK options](SDK.md#proxy-options).

For an embedded application, call `GenerateDeviceprint` once and retain the identity explicitly. Pass its value in `AuthOptions.Deviceprint`; `GenerateAntifraudDeviceprint(device.Value())` derives the separate wire form. These are protocol identities, not observed browser-compatibility evidence. An existing observed bundle can use `NewPrimaryAuthFromBundle`.

Primary state machine:

1. Create `NewPrimaryAuth(options)` and arrange `Close()` on every exit.
2. Call `Login(ctx, ownerLogin, ownerPassword, PrimaryLoginOptions{})`.
3. On `*PinOTPRequired` through `errors.As`, obtain the owner's code and call `ConfirmOTP` once.
4. A successful nil bundle means enrollment is required: obtain and confirm the owner's new PIN, then call `CreatePIN`. Nil is not an authenticated session.
5. Save a validated returned bundle to an explicit private destination with `SessionBundle.Save`. CLI first publication additionally enforces no-replace and cleanup before publication.

`CaptchaAnswer` accepts deliberate caller-supplied answers. Resend methods exist for owner-selected retries; they are not automatic. Diagnostics redact identity/cookies; explicit persistence is the credential boundary.

Remembered-device login: create `NewPINAuthFromProfile(path, options)`, call `Login(ctx, ownerPIN, CaptchaAnswer{})`, handle `PinOTPRequired` with `ConfirmOTP`, save the refreshed bundle and close auth. `NewSberClientFromPINProfile` offers ordinary renewal with a `PINProvider`; required OTP/CAPTCHA return to the caller for explicit handling.

The CLI also supports remembered-device login without changing the existing PIN:

```sh
./bin/sber login --remembered-profile "$HOME/.config/sber-sdk/profile.json" \
  --profile "$HOME/.config/sber-sdk/profile-next.json"
```

It reads the configured online-banking PIN or asks for it at a hidden prompt, and requests one terminal OTP only if required. It does not ask for primary credentials or create another PIN. The source profile is read without replacement; the new destination must be absent. Missing source, cleanup failure and rejected authentication prevent publication. Existing-profile CLI reads with a terminal or configured PIN can renew once and retry one read. `--no-renew`, MCP, export, credential inspection, and mutation commands do not renew. Batch reads without a configured PIN do not prompt for renewal. MCP remains separate from terminal input.

`refresh-session --profile EXISTING_PATH` restores and atomically updates an existing full remembered profile. With `SBER_PINCODE` in the environment or selected env file, it needs no terminal unless the bank requests SMS. A required SMS without a terminal produces a fixed instruction and preserves the old profile. Authentication and cleanup complete before publication; failures retain the old profile. Primary and remembered CLI authentication load public configuration before consuming secret values. Definite new-PIN policy rejections can request a fresh interactive owner choice in the same process, at most three bank attempts; no fixed environment PIN, password/SMS or uncertain PIN request is replayed.

All authentication paths now use native Go HTTP requests. No Playwright, Firefox, Chrome or JavaScript runtime is linked or launched. The SDK exposes separate `PhoneAuth`, `CardAuth`, and `QRAuth` state machines. The old `browser` package, browser bootstrap options, and CLI browser flags have been removed. Existing HTTP session profiles retain their cookie and device metadata.

Native authentication declares `Mozilla/5.0 (compatible; sber-go/1.0; +https://github.com/vasyza/sber-go)` as its default User-Agent. The compatibility prefix is required by the bank web-session handoff. This header identifies the SDK and contains no browser-engine claim. Explicit supplied User-Agent headers remain unchanged. The successful session stores this header for subsequent native requests.

Phone authentication uses `/uapi/v2/authenticate` for SRP identification and `/uapi/v2/verify` for the client proof and optional SMS. It checks the server SRP proof before accepting a challenge or redirect. An explicit `password_bypass` response can request the bank-defined password transition. The client uses the returned OUID and RSA-OAEP key for `encodedPassword`, or the TLS password method when the bank omits a key. A false SRP proof or an uncertain response never triggers this transition. The returned host must pass the same HTTPS bank-origin policy as primary and PIN authentication. `AuthToken` is encoded once in the final redirect.

Card authentication encrypts the PAN with the public configuration key and its key identifier. `/api/v1/cardlogin/identify` starts SMS confirmation. `/api/v1/cardlogin/confirm` returns optional web PIN enrollment. `CreatePIN` and the final `/api/v1/auth` transition share the primary implementation. The SDK does not request a card PIN, CVV, or expiry date. Account setup or password recovery is an explicit unsupported continuation, never an automatic password reset.

QR authentication calls `/uapi/v2/identify`, then polls `/uapi/v2/getOperation`. HTTP 204 means that no new status is available. A confirmed operation supplies the OUID for `/uapi/v2/verify` with `pat_check`. New, waiting, refused, expired, and confirmed states are distinct. Unknown states fail closed. The CLI waits up to three minutes and never rotates a QR automatically. QR contents have redacted formatting; `Content()` is an explicit secret read for display to the owner.

Primary and PIN authentication retain the native HTTP protocol and no-replay policy. If the bank returns a browser security check, native authentication stops. It does not synthesize a protection cookie or run a fallback browser.

The bank can also return a connection rejection page with HTTP 200 and no configuration. Authentication reports `login_page_rejected` before it sends credentials to the bank. All login methods use this public configuration step. The error does not identify the cause of the bank rejection. Select a connection or explicit proxy that the bank accepts; native authentication does not change its client identity to hide this refusal.

HAR import is offline and bounded to 2 MiB. A successful seamless response's single validated `X-Response-URL` can identify `/main`, while strict runtime HTML identifies the API origin. SameSite policy casing is normalized without changing its meaning. Chromium's exact session-expiry sentinel is preserved as an unset expiry; explicit deletion still takes priority. See the [DevTools HAR writer](https://github.com/ChromeDevTools/devtools-frontend/blob/main/front_end/models/har/Log.ts) and [protocol cookie definition](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Network.pdl). Import success alone does not prove bank authorization.

On 2026-10-07, native remembered PIN login and the real authenticated E2E below passed on macOS, including a second session created through the CLI's production PIN factory. The successful flow used ordinary public browser initialization and observed identity. A locally imported Yandex HAR was not accepted by the authenticated bank probe (HTTP 403), and cold primary login without observed browser initialization previously failed at final navigation; neither is counted as successful live authorization. Exact sums/account selection were not independently reconciled with the bank UI. [STATUS.md](STATUS.md) records the validation boundary.

After owner login, the separately enabled integration test checks authorization, products and one page (at most five returned operations) for the last seven days. It prints only success stages and counts. It does not initiate payments, renew through PIN/OTP, export responses or prove complete history coverage. Default tests do not compile or execute it:

```sh
go test -tags=live -run '^TestLive(CLIReadOnly|ReadOnly)$' -count=1 -v ./tests/integration \
  -args -sber-live -sber-profile "$HOME/.local/share/sber-go/profile.json"
```

The native CLI test additionally covers all data-read commands, offline metadata, credential metadata, and private session export. It supplies `--no-renew`, holds identifiers/results in transient memory, and removes the temporary private export. Running `go test -tags=live ./tests/integration` without `-sber-live` compiles and skips both tests before opening a profile, building a temporary binary, or contacting the bank. Session cookies may rotate and be saved back to the explicitly selected private profile during authenticated reads.
