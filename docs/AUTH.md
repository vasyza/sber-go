# Authentication and profiles

On Linux and macOS the native owner path is `./bin/sber login --profile "$HOME/.local/share/sber-go/profile.json"`. It acquires an enrollment lock before prompts, creates missing private parents and refuses an existing profile. Login/password are hidden, requested OTP is handled once, and online-PIN enrollment asks for confirmation. Password/PIN/OTP are not profile fields. Echo-control failure stops input. CAPTCHA/WebAuthn are not automated CLI flows.

macOS uses the terminal device path returned by the kernel, verified against the original input descriptor, and native exclusive rename for first publication. Linux uses its pinned procfs descriptor capabilities. Both restore terminal settings after success, failure or cancellation and preserve an existing profile. Profile paths must have literal, symlink-free components; use your home directory rather than macOS `/tmp` or `/var` aliases.

Native SDK, CLI and MCP requests use a verified Russian Trusted Root CA embedded in the build. Bank certificate trust needs no external file or runtime download, including when a system PEM bundle is unavailable. Default trust also retains a canonical system PEM bundle when present; `SSL_CERT_FILE` and `SSL_CERT_DIR` do not select alternative trust. TLS chain, validity and hostname verification remain enabled, and system trust stores are not changed. See [certificate provenance and rotation](../internal/transport/certificates/README.md).

An optional `--ca-bundle PATH` replaces default trust for that client. Embedded callers use `AuthOptions.TransportOptions.CABundle` or `ClientOptions.TransportOptions.CABundle`. An invalid explicit CA file fails before credential input and does not fall back to the embedded root. The CLI reports safe failure classifications without emitting raw bank messages, credentials or response bodies.

PIN enrollment announces the length from the validated live configuration. Invalid local digit input or a confirmation mismatch can be corrected within the same authentication process, without repeating login/OTP or submitting another PIN creation request. The final seamless navigation retains the originating `Process-Id`, matching the bank's public `r-97.0.0` login client contract.

Native authentication retains verified HTTP/1.1 connections across its steps. It rejects replay-enabling POST headers and never repeats a transmitted POST after an uncertain response, including the empty seamless-navigation POST. Business transports keep one request per connection. `TransportOptions.Timeout` bounds the complete request, including connection establishment, TLS verification and response reading; transport phases do not silently shorten this budget.

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

It asks for the existing online-banking PIN and one OTP only if required. It does not ask for primary credentials or create another PIN. The source profile is read without replacement; the new destination must be absent. Missing source, cleanup failure and rejected authentication prevent publication. Existing-profile CLI reads do not prompt for renewal. MCP remains separate from terminal input.

When an observed browser identity/cookie initialization is required, select ordinary public rendering explicitly:

```sh
./bin/sber login --profile /absolute/private/new-profile.json \
  --browser-profile /absolute/private/dedicated-firefox-profile \
  --playwright-driver /absolute/installed/matching-playwright-driver \
  --firefox-executable /absolute/installed/matching-firefox
```

These three browser paths must all be absolute. They can also accompany `--remembered-profile`. Provision the matching official Playwright runtime separately; the command never installs or discovers a browser implicitly. The dedicated browser directory must already be private (0700), owned by the caller and contain verified NSS certificate trust. Native Go trust, whether embedded or explicitly overridden with `--ca-bundle`, does not provision Firefox NSS. The browser provider keeps certificate/hostname verification and the sandbox enabled, renders the public login document only, and never receives login/password/PIN/OTP. The auth state machine atomically adopts validated rendered configuration, cookies and observed browser identity before native credential requests. Embedded callers select `AuthOptions.BrowserFirst` and `BrowserBootstrap` explicitly.

The matching runtime installation is documented in the official [Playwright Go project](https://github.com/mxschmitt/playwright-go). The provider expects the driver/browser version matching the dependency in `go.mod`.

HAR import is offline and bounded to 2 MiB. A successful seamless response's single validated `X-Response-URL` can identify `/main`, while strict runtime HTML identifies the API origin. SameSite policy casing is normalized without changing its meaning. Chromium's exact session-expiry sentinel is preserved as an unset expiry; explicit deletion still takes priority. See the [DevTools HAR writer](https://github.com/ChromeDevTools/devtools-frontend/blob/main/front_end/models/har/Log.ts) and [protocol cookie definition](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Network.pdl). Import success alone does not prove bank authorization.

On 2026-10-07, native remembered PIN login and the real authenticated E2E below passed on macOS, including a second session created through the CLI's production PIN factory. The successful flow used ordinary public browser initialization and observed identity. A locally imported Yandex HAR was not accepted by the authenticated bank probe (HTTP 403), and cold primary login without observed browser initialization previously failed at final navigation; neither is counted as successful live authorization. Exact sums/account selection were not independently reconciled with the bank UI. [STATUS.md](STATUS.md) records the validation boundary.

After owner login, the separately enabled integration test checks authorization, products and one page (at most five returned operations) for the last seven days. It prints only success stages and counts. It does not initiate payments, renew through PIN/OTP, export responses or prove complete history coverage. Default tests do not compile or execute it:

```sh
go test -tags=live -run '^TestLiveReadOnly$' -count=1 -v ./integration \
  -args -sber-live -sber-profile "$HOME/.local/share/sber-go/profile.json"
```

Running `go test -tags=live ./integration` without `-sber-live` compiles and skips the test without opening a profile or contacting the bank. Session cookies may rotate and be saved back to the explicitly selected private profile during authenticated reads.
