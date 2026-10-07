# Authentication and profiles

On Linux the native owner path is `./bin/sber login --profile "$HOME/.local/share/sber-go/profile.json"`. It acquires an enrollment lock before prompts, creates missing private parents and refuses an existing profile. Login/password are hidden, requested OTP is handled once, and online-PIN enrollment asks for confirmation. Password/PIN/OTP are not profile fields. Echo-control failure stops input. CAPTCHA/WebAuthn are not automated CLI flows.

For an embedded application, call `GenerateDeviceprint` once and retain the identity explicitly. Pass its value in `AuthOptions.Deviceprint`; `GenerateAntifraudDeviceprint(device.Value())` derives the separate wire form. These are protocol identities, not observed browser-compatibility evidence. An existing observed bundle can use `NewPrimaryAuthFromBundle`.

Primary state machine:

1. Create `NewPrimaryAuth(options)` and arrange `Close()` on every exit.
2. Call `Login(ctx, ownerLogin, ownerPassword, PrimaryLoginOptions{})`.
3. On `*PinOTPRequired` through `errors.As`, obtain the owner's code and call `ConfirmOTP` once.
4. A successful nil bundle means enrollment is required: obtain and confirm the owner's new PIN, then call `CreatePIN`. Nil is not an authenticated session.
5. Save a validated returned bundle to an explicit private destination with `SessionBundle.Save`. CLI first publication additionally enforces no-replace and cleanup before publication.

`CaptchaAnswer` accepts deliberate caller-supplied answers. Resend methods exist for owner-selected retries; they are not automatic. Diagnostics redact identity/cookies; explicit persistence is the credential boundary.

Remembered-device login: create `NewPINAuthFromProfile(path, options)`, call `Login(ctx, ownerPIN, CaptchaAnswer{})`, handle `PinOTPRequired` with `ConfirmOTP`, save the refreshed bundle and close auth. `NewSberClientFromPINProfile` offers ordinary renewal with a `PINProvider`; required OTP/CAPTCHA return to the caller for explicit handling.

Existing-profile CLI reads do not prompt for renewal. On expired authorization perform explicit SDK renewal or create a new owner profile. MCP remains separate from terminal input. Linux/macOS support SDK loading and atomic saves; hidden CLI enrollment is Linux-only.

Owner smoke sequence: `check-session`, `products`, one `operations-page`, then compare account selection, exact sums, dates and coverage metadata with the bank UI. This sequence was not performed against a real account during repository repair.
