# Complete migration acceptance matrix

Current runtime adjustment (2026-10-07): the observed bank frontend sends `cardIds` as JSON numbers for `/ufs-carddetail/rest/card/v1/cardInfo`. The native `Cards.Info` and `Cards.Limits` methods now use checked `int64` values. The original string-array contract produced HTTP 500. The frozen source excerpts in `parity.json` remain unchanged as historical reference; the current command contract is in [CLI-REFERENCE.md](CLI-REFERENCE.md).

**Every acceptance row is `pending`: none is declared implemented in Go.** `docs/parity.json` is the machine-authoritative matrix; this document is its readable surface and safety-seam index. Future `TestParity…`/`FuzzParity…` identifiers are required tests, not claims that they exist or pass.

## Frozen authority and boundaries

- Python root: `/home/hermes/workspace/rental-monitoring/sber-sdk/fork`.
- HEAD: `984d45ae6ddbf37224e8146df2788e8821108c9e` **plus the 22 audited staged repairs**, including browser bootstrap and SameSite changes. Working-tree file hashes and the staged-file list are in `docs/source-manifest.json`.
- Scope: all 14 SDK modules, SDK CLI, deprecated read/parser shims, the local safe launcher, all MCP implementation/demo modules, and all 30 synthetic test modules. This is not a rental-history-only port.
- Native target: `github.com/vasyza/sber-go`. Python is reference material only; no runtime subprocess adapter.
- No bank/network/authentication call, private-owner artifact read, Go source edit, repo API write, commit or publication occurred in this inventory task.
- Read-only defaults do **not** narrow the bank session privilege. Full mutation parity remains required behind explicit policy. Partial/unavailable history means UNKNOWN, never unpaid.
- Source excerpts and fixtures retain **MIT — Copyright (c) 2026 sber-mcp contributors**. The original notice is stored in the manifest and fixture README.

## Programmatically verified coverage

Counts were checked by rereading the persisted JSON catalogs and independently parsing the source AST. The raw AST name count includes six lexically nested callback/helper names; these are retained but are not exposed APIs. Private protocols/mixins and special constructors/context/repr methods are explicitly inventoried, not silently omitted.

| Inventory | Verified count |
|---|---:|
| Acceptance rows, all pending | 1109 |
| Unique required Go test identifiers in matrix | 1147 |
| Scoped public-named callables | 162 |
| Raw AST public-named functions including nested callbacks | 168 |
| All source lexical callable definitions, including helpers | 491 |
| Additional inherited public methods | 12 |
| Class/type definitions | 80 |
| Declared class fields | 209 |
| Explicit export mappings | 177 |
| Protocol/default/metadata constants | 94 |
| Endpoint contracts | 23 |
| CLI command-surface rows (some group aliases) | 14 |
| MCP tools in full opt-in surface | 14 |
| MCP default tools / write-gated tools / demo tools | 9 / 5 / 6 |
| Synthetic test definitions | 199 |
| Expanded parameter cases across those definitions | 324 |
| Runtime tests / separate maintenance tests | 183 / 16 |
| Test-derived golden/vector records | 35 |
| Cross-layer safety/compatibility seams | 19 |

Missing public callables: **0**. Missing MCP definitions: **0**. Missing test definitions: **0**. Every recorded source hash still matched the audited working tree.

`input_contract`, `output_contract`, `error_contract`, `state_contract`, `permission_contract`, exact signature/decorators, source file/lines, AST predicates/raises/handlers/return expressions/request expressions, helper links and required Go tests live on the individual JSON rows. A method with no direct `raise` still inherits its helper/parser/transport/persistence failures; it is not an error-free promise.

## Read resources versus bank mutations

POST is **not** a financial classification: products/history/details/card info/PFM and warmup use POST but are read/session operations. Transfer start/prepare mutate provider workflow state even before money movement. SDK `transfer_to` only prepares; explicit confirmation remains separate.

| Effect | HTTP | Endpoint | Input/output contract | Source | Required Go test |
|---|---|---|---|---|---|
| read | POST | `/main-screen/rest/v2/m1/web/section/meta` | {"withData":true,"forceUpdate":"bool(force_update)"} → Products(accounts:tuple[Account],cards:tuple[Card]) with all three account sections and wallet cards | `sber_unofficial/resources.py:91–97` | `TestParityEndpointProducts` |
| read | POST | `/uoh-bh/v1/operations/list` | {"paginationOffset":"offset >=0","paginationSize":"limit+1 (limit1..100)","showHidden":false,"showNotTransactionBonuses":true,"showOpenBanking":true,"from":"optional dd.mm.YYYYTHH:MM:SS Moscow; frozen120day default","to":"optional same format, date end23:59:59","usedResource":"optional [resource]"} → OperationsPage N+1 sentinel semantics; missing uohId hard failure | `sber_unofficial/resources.py:112–154` | `TestParityEndpointOperations` |
| read | POST | `/uoh-bh/v1/operation/details` | {"uohId":"[A-Za-z0-9_-]{1,128}"} → OperationDetail and all Money\|string\|null detail fields | `sber_unofficial/resources.py:209–214` | `TestParityEndpointOperationDetails` |
| read | POST | `/ufs-carddetail/rest/card/v1/cardInfo` | {"cardIds":"nonempty list of normalized positive JS-safe numeric strings"} → tuple[CardInfo], full limits/optional credit terms; last4 PAN only | `sber_unofficial/resources.py:245–256` | `TestParityEndpointCardInfo` |
| read | POST | `/pfpv_alf_mb/v1.00/alf/amounts` | {"filter":{"from":"ISO+03:00","to":"ISO+03:00 date end","incomeType":"income\|outcome","betweenOwnFilter":"on\|off","openBankingFilter":"on\|off","productFilters":[{"type":"CARD","filter":"custom"},{"type":"CT_ACCOUNT","filter":"custom"},{"type":"MANUAL","filter":"custom"}]},"display":{"showCategoryAmounts":"bool","showProductAmounts":"bool"}} → PfmAmounts periods/category totals; source public model does not expose productAmounts | `sber_unofficial/resources.py:265–300` | `TestParityEndpointPfmAmounts` |
| session_keepalive | POST | `/api/warmUpSession` | {} → 200/204 non-HTML => empty mapping; persist rotation/discovery after success | `sber_unofficial/client.py:239–253` | `TestParityEndpointWarmUp` |
| nonfinancial_bank_mutation | POST | `/ufs-productdetail/rest/v1/changeProductName` | {"id":"positive JS-safe integer","name":"allowed <=56 char string","type":"card"} → None after definite success | `sber_unofficial/resources.py:224–243` | `TestParityEndpointCardRename` |
| financial_workflow_mutation | POST | `/me2me/v1/workflow` | {"steps":[{"query":{"cmd":"START","name":"me2meMain_v2"},"json":{"document":{"action":"CREATE"}}},{"query":{"cmd":"EVENT","name":"next","pid":"draft.pid"},"json":{"fields":["transfer:me2me:fromResource","transfer:me2me:toResource","transfer:me2me:sum","transfer:me2me:sum:currency","transfer:me2me:paymentPurpose"],"document":{"flow":"me2meCreate","state":"transferRequisites"}}},{"query":{"cmd":"EVENT","name":"summaryNext","pid":"prepared.pid"},"json":{"document":{"flow":"me2meCreate","state":"summary"}}},{"query":{"cmd":"EVENT","name":"on-return","pid":"prepared.pid"},"json":{}}]} → TransferDraft → PreparedTransfer → external workflow transitions → TransferResult with done marker+document | `sber_unofficial/resources.py:311–493` | `TestParityEndpointOwnResourceWorkflow` |
| financial_confirmation_mutation | POST | `/bh-confirmation/v3/workflow2` | {"query":{"cmd":"EVENT","name":"on-enter","pid":"prepared.pid"},"json":{}} → EXTERNAL_RETURN with exact /me2me/v1/workflow; no unsupported OTP transition | `sber_unofficial/resources.py:435–470` | `TestParityEndpointConfirmationWorkflow` |
| auth_session_mutation | POST_JSON | `/api/v1/pin/begin` | {"srp_A":"minimal lowercase hex","deviceprint":"opaque exact string","isPwa":"optional true","captchaCode":"optional code","audioCaptchaCode":"optional code"} → srp_B and srp_s hex; CSRF rotates even on failure | `sber_unofficial/pin.py:229–306` | `TestParityEndpointPinBegin` |
| auth_session_mutation | POST_JSON | `/api/v1/pin/logon` | {"srp_M":"M1 hex","deviceprint":"opaque","isPwa":"optional true"} → srp_R verified then redirect or confirmInfo/lifetime/attempts | `sber_unofficial/pin.py:275–302` | `TestParityEndpointPinLogon` |
| auth_session_mutation | POST_JSON | `/api/v1/pin/otp/confirm` | {"confirmPassword":"code1..32 no controls","deviceprint":"opaque","isPwa":"optional true"} → validated redirect finishes session | `sber_unofficial/pin.py:308–322` | `TestParityEndpointPinOtpConfirm` |
| auth_session_mutation | POST_JSON | `/api/v1/pin/otp/retry` | {"isPwa":"only field when true; otherwise {}"} → PinOtpRequired return with updated metadata | `sber_unofficial/pin.py:324–337` | `TestParityEndpointPinOtpRetry` |
| auth_session_mutation | POST_FORM | `/authMainJson.do` | {"content_type":"application/x-www-form-urlencoded","operations":{"button.begin":["login","pageInputType=INDEX","storeLogin=true\|false","srp_A","publicKeyCredentialAvailable=true\|false","optional token/captchaCode/audioCaptchaCode","deviceprint","jsEvents=","domElements="],"button.next_proof":["login","srp_M","token","org.apache.struts.taglib.html.TOKEN","storeLogin","pageInputType=INDEX","context"],"button.next_otp":["confirmPassword","token","org.apache.struts.taglib.html.TOKEN","pageInputType=INDEX","context"],"button.getNewPass":["token","org.apache.struts.taglib.html.TOKEN","pageInputType=INDEX","context"]}} → nested SRP proof; NEED_CONFIRM/WRONG_PASS/ATTEMPTS_EXHAUSTED/NOT_REQUIRED, pinInfo publicKey/skip or redirect | `sber_unofficial/pin.py:703–841` | `TestParityEndpointPrimaryAuthForm` |
| auth_session_mutation | POST_JSON | `/api/v1/pin/create` | {"pin":"Base64 RSA-OAEP SHA1/MGF1-SHA1 encrypted PIN","deviceprint":"opaque","isPwa":"optional true"} → PIN accepted; discard spent key then finish auth | `sber_unofficial/pin.py:843–875` | `TestParityEndpointPinCreate` |
| auth_session_mutation | POST_JSON | `/api/v1/auth` | {"deviceprint":"opaque","isPwa":"optional true"} → validated redirect required then UFS session | `sber_unofficial/pin.py:995–1003` | `TestParityEndpointAuthFinish` |
| public_bootstrap_read | GET | `https://online.sberbank.ru/CSAFront/index.do` | {"headers":"document navigation; no origin/content-type/X-Requested-With"} → strict literal config, or recognized TSPD => opt-in renderer; complete state only after validation | `sber_unofficial/pin.py:374–445` | `TestParityEndpointAuthPublicPage` |
| session_discovery_read | GET | `<APP_ORIGIN>/app/main` | {} → unique HTTPS online ufsHost origin | `sber_unofficial/client.py:416–447` | `TestParityEndpointAppShell` |
| session_discovery_read | GET | `<web_base>/main` | {} → unique HTTPS online ufs.block.root.url origin | `sber_unofficial/client.py:416–447` | `TestParityEndpointUfsMain` |
| session_ready_read | GET | `<web_base>/api/front/ready` | {} → HTTP200 readiness when config.seamless_web=true | `sber_unofficial/pin.py:620–632` | `TestParityEndpointSeamlessReady` |
| auth_session_navigation | GET or bodyless POST | `validated dynamic redirect/Location URL within HTTPS online hosts` | {"json":null,"post_redirect":"config.redirect_post; only307/308 preserves POST"} → max10-step navigation to final/main or app shell then UFS; session auth cookies ready | `sber_unofficial/pin.py:534–618` | `TestParityEndpointAuthRedirect` |
| auth_challenge_asset_read | GET | `validated captcha URL under frontend auth base path` | {} → nonempty bytes <=10MiB HTTP200 | `sber_unofficial/pin.py:339–364` | `TestParityEndpointHumanCaptchaAsset` |
| public_bootstrap_read | GET | `approved GET TSPD/static assets under HTTPS online hosts` | {"allow":"root exact AUTH_PAGE; child/tspd; first-party static extensions"} → ordinary rendered literal HTML+snapshot only | `sber_unofficial/bootstrap.py:195–203` | `TestParityEndpointOrdinaryBrowserAssets` |

## All MCP tools, including gated mutations

No tool accepts login/password/PIN/OTP/captcha answer/deviceprint/cookies/UFS credentials. Confirmation tokens are local expiring approval handles, not bank authentication tokens. All definitions are counted from both `_TOOLS` and top-level `async def sber_*`, not from a stale comment.

| Tool | Exact parameters | Default policy | Effect | Source | Required Go test |
|---|---|---|---|---|---|
| `sber_setup_status` | `profile: str \| None=None` | default and demo | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:831–869` | `TestParity_CallableMcpSberUnofficialMcpServerSberSetupStatusL831` |
| `sber_auth_start` | `mode: str='auto', profile: str='default'` | default; absent in demo | auth_session_mutation | `mcp/sber_unofficial_mcp/server.py:898–918` | `TestParity_CallableMcpSberUnofficialMcpServerSberAuthStartL898` |
| `sber_auth_continue` | `session_id: str \| None=None, wait_seconds: int=30, use_audio_captcha: bool=False` | default; absent in demo | auth_session_mutation | `mcp/sber_unofficial_mcp/server.py:1054–1085` | `TestParity_CallableMcpSberUnofficialMcpServerSberAuthContinueL1054` |
| `sber_auth_resend_otp` | `session_id: str \| None=None` | default; absent in demo | auth_session_mutation | `mcp/sber_unofficial_mcp/server.py:1170–1182` | `TestParity_CallableMcpSberUnofficialMcpServerSberAuthResendOtpL1170` |
| `sber_session_info` | `session_id: str \| None=None, check_live: bool=False` | default and demo | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:1185–1208` | `TestParity_CallableMcpSberUnofficialMcpServerSberSessionInfoL1185` |
| `sber_session_close` | `session_id: str \| None=None` | default and demo | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:1211–1219` | `TestParity_CallableMcpSberUnofficialMcpServerSberSessionCloseL1211` |
| `sber_products` | `session_id: str \| None=None, force_update: bool=False` | default and demo | read | `mcp/sber_unofficial_mcp/server.py:1228–1243` | `TestParity_CallableMcpSberUnofficialMcpServerSberProductsL1228` |
| `sber_operations` | `session_id: str \| None=None, resource: str \| None=None, from_date: str \| None=None, to_date: str \| None=None, limit: int=30, max_pages: int=3` | default and demo | read | `mcp/sber_unofficial_mcp/server.py:1252–1275` | `TestParity_CallableMcpSberUnofficialMcpServerSberOperationsL1252` |
| `sber_operations_page` | `session_id: str \| None=None, resource: str \| None=None, offset: int=0, limit: int=30, from_date: str \| None=None, to_date: str \| None=None` | default and demo | read | `mcp/sber_unofficial_mcp/server.py:1278–1296` | `TestParity_CallableMcpSberUnofficialMcpServerSberOperationsPageL1278` |
| `sber_card_rename` | `card_id: str, name: str, session_id: str \| None=None` | opt-in only; absent in demo | nonfinancial_bank_mutation | `mcp/sber_unofficial_mcp/server.py:1299–1321` | `TestParity_CallableMcpSberUnofficialMcpServerSberCardRenameL1299` |
| `sber_transfer_start` | `session_id: str \| None=None` | opt-in only; absent in demo | financial_workflow_mutation | `mcp/sber_unofficial_mcp/server.py:1324–1344` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferStartL1324` |
| `sber_transfer_prepare` | `draft_id: str, source_id: str, destination_id: str, amount: str, currency: str='RUB', payment_purpose: str='', session_id: str \| None=None` | opt-in only; absent in demo | financial_workflow_mutation | `mcp/sber_unofficial_mcp/server.py:1347–1396` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferPrepareL1347` |
| `sber_transfer_confirm` | `confirmation_token: str, acknowledged_amount: str, acknowledged_destination_id: str, session_id: str \| None=None` | opt-in only; absent in demo | financial_confirmation_mutation | `mcp/sber_unofficial_mcp/server.py:1399–1461` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferConfirmL1399` |
| `sber_transfer_resolve_uncertain` | `audit_id: str, found: bool, session_id: str \| None=None` | opt-in only; absent in demo | financial_control_reconciliation | `mcp/sber_unofficial_mcp/server.py:1464–1478` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferResolveUncertainL1464` |

## CLI surfaces

| Surface / command | Inputs | Outputs | Safety/error contract | Source | Required Go test |
|---|---|---|---|---|---|
| cli:sdk_cli:import-session | {"har":"positional Path","--cookies":"required Path","--output":"required Path"} | offline private SessionBundle plus safe summary | ["no secret argument/env fallback introduced","Go owner CLI uses fail-closed hidden TTY safeguards from local_sber, including login/OTP/captcha","no bank mutation commands"]; ["argparse/handled SDK/OS/value/EOF/cancel => exit2","network/auth never part of this inventory run"] | `sber_unofficial/__main__.py:38–44` | `TestParity_CLISberUnofficialMainImportSession` |
| cli:sdk_cli:inspect-session | {"session":"positional private Path"} | offline redacted metadata only | ["no secret argument/env fallback introduced","Go owner CLI uses fail-closed hidden TTY safeguards from local_sber, including login/OTP/captcha","no bank mutation commands"]; ["argparse/handled SDK/OS/value/EOF/cancel => exit2","network/auth never part of this inventory run"] | `sber_unofficial/__main__.py:46–50` | `TestParity_CLISberUnofficialMainInspectSession` |
| cli:sdk_cli:pin-login | {"cookies":"private Chrome JSON","--output":"required","--deviceprint-file":"required private UTF8","--har":"optional browser identity HAR","--captcha-output":"optional exclusive artifact"} | owner interactive remembered PIN/OTP/captcha; save0600 session | ["no secret argument/env fallback introduced","Go owner CLI uses fail-closed hidden TTY safeguards from local_sber, including login/OTP/captcha","no bank mutation commands"]; ["argparse/handled SDK/OS/value/EOF/cancel => exit2","network/auth never part of this inventory run"] | `sber_unofficial/__main__.py:52–65` | `TestParity_CLISberUnofficialMainPinLogin` |
| cli:sdk_cli:enroll | {"--output":"required","--deviceprint-file":"required private","--har":"optional","--captcha-output":"optional"} | owner primary login/password/SMS/newPIN or direct redirect; private profile | ["no secret argument/env fallback introduced","Go owner CLI uses fail-closed hidden TTY safeguards from local_sber, including login/OTP/captcha","no bank mutation commands"]; ["argparse/handled SDK/OS/value/EOF/cancel => exit2","network/auth never part of this inventory run"] | `sber_unofficial/__main__.py:67–79` | `TestParity_CLISberUnofficialMainEnroll` |
| cli:sdk_cli:balances | {"session":"private positional","--force-update":"bool","--output":"required","--save-session":"optional","--pin-keychain-service":"paired optional","--pin-keychain-account":"paired optional"} | Products private JSON; file path only to stdout | ["no secret argument/env fallback introduced","Go owner CLI uses fail-closed hidden TTY safeguards from local_sber, including login/OTP/captcha","no bank mutation commands"]; ["argparse/handled SDK/OS/value/EOF/cancel => exit2","network/auth never part of this inventory run"] | `sber_unofficial/__main__.py:81–87` | `TestParity_CLISberUnofficialMainBalances` |
| cli:sdk_cli:operations | {"session":"private positional","--resource":"optional","--from":"optional ISO date","--to":"optional ISO date","--output":"required","--save-session":"optional","--pin-keychain-service":"paired optional","--pin-keychain-account":"paired optional"} | complete deduped operations private JSON; file path stdout | ["no secret argument/env fallback introduced","Go owner CLI uses fail-closed hidden TTY safeguards from local_sber, including login/OTP/captcha","no bank mutation commands"]; ["argparse/handled SDK/OS/value/EOF/cancel => exit2","network/auth never part of this inventory run"] | `sber_unofficial/__main__.py:89–98` | `TestParity_CLISberUnofficialMainOperations` |
| cli:safe_launcher:status | {"command":"status","args_secret_fields":[]} | {"sdk_version":"reference metadata","profile_exists":"file existence only","bank_authorization_checked":false,"exit":0} | ["private stable lock, no-replace publication for enrollment","hidden terminal only","no MCP or payment commands"]; ["no bank check"] | `../local_sber.py:124–162` | `TestParity_CLILocalSberStatus` |
| cli:safe_launcher:doctor | {"command":"doctor","args_secret_fields":[]} | {"stage":"unauthenticated_bootstrap","bank_login_attempted":false,"ready":"bool","exit_success":0,"exit_failure":3} | ["private stable lock, no-replace publication for enrollment","hidden terminal only","no MCP or payment commands"]; ["allowlisted fixed error code or safe class name"] | `../local_sber.py:124–162` | `TestParity_CLILocalSberDoctor` |
| cli:safe_launcher:login | {"command":"login","args_secret_fields":[]} | {"stage":"authorization","ready":"bool","exit_success":0,"exit_failure":3} | ["private stable lock, no-replace publication for enrollment","hidden terminal only","no MCP or payment commands"]; ["non-TTY=>exit2 before any secret","existing profile or symlink=>refuse","no public bootstrap=>no secret prompt"] | `../local_sber.py:124–162` | `TestParity_CLILocalSberLogin` |
| cli:mcp_cli:login\|setup\|enroll | {"--profile":"[a-z0-9_-]{1,32}, default default"} | localized owner-only auth, stable synthetic fingerprint files and saved profile | ["hidden owner terminal input must fail closed","demo no bank","mutation registration absent unless explicit opt-in"]; ["optional MCP import error explains install; no invented adapter","static/localized auth error codes","cancel/EOF safe exit"] | `mcp/sber_unofficial_mcp/server.py:1987–2010` | `TestParity_CLIMcpSberUnofficialMcpServerLoginSetupEnroll` |
| cli:mcp_cli:status | {"profile":"SBER_MCP_PROFILE or default"} | offline artifact metadata; no secrets | ["hidden owner terminal input must fail closed","demo no bank","mutation registration absent unless explicit opt-in"]; ["optional MCP import error explains install; no invented adapter","static/localized auth error codes","cancel/EOF safe exit"] | `mcp/sber_unofficial_mcp/server.py:1987–2010` | `TestParity_CLIMcpSberUnofficialMcpServerStatus` |
| cli:mcp_cli:serve\|run\|default | {"--allow-writes":"opt-in; SBER_MCP_ALLOW_WRITES also","--demo":"no bank; SBER_MCP_DEMO overrides writes"} | stdio JSONRPC only; TTY notice stderr; default9 tools/demo6/full14 | ["hidden owner terminal input must fail closed","demo no bank","mutation registration absent unless explicit opt-in"]; ["optional MCP import error explains install; no invented adapter","static/localized auth error codes","cancel/EOF safe exit"] | `mcp/sber_unofficial_mcp/server.py:1987–2010` | `TestParity_CLIMcpSberUnofficialMcpServerServeRunDefault` |
| cli:legacy_cli:inspect-har\|balances\|operations | {"har":"positional","--cookies":"optional","--output":"required for network","--force-update/--resource/--from/--to":"as declared"} | explicit native-Go compatibility mapping for offline inspection and read commands | ["never ship Python runtime dependency","no financial mutation"]; ["legacy synchronous decoder/warmup/sorting differences require explicit adaptation"] | `sber_client.py:329–376` | `TestParity_CLISberClientLegacyCommands` |
| cli:offline_har_cli:parse\|self-test | {"har":"optional positional","--self-test":"bool"} | HAR cached response→<file.har>.sber.json0600 or self-test ok | ["offline only","no copying owner HAR into repo"]; ["missing HAR argparse error; strict parser errors"] | `sber_unofficial/har.py:257–272` | `TestParity_CLISberUnofficialHarParseSelftest` |

The safe launcher’s hidden-input/bootstrap-first/private-lock/no-replace rules supersede SDK/MCP echo-prone login/getpass fallbacks. This is an explicit security adaptation, not a dropped login/PIN/OTP/captcha feature.

## Public callable acceptance index

This includes public-named members on protocol/private mixin classes because they define inherited/public behavior. Constructors, validation hooks, context methods, repr contracts, all internal definitions and nested callbacks are separately present in JSON. Each row below remains `pending`.

| Category | Public callable / signature | Effect | Source | Required Go test |
|---|---|---|---|---|
| sdk_cli | `main`<br>`def main() -> None` | cli_auth_or_read_dispatch | `sber_unofficial/__main__.py:336–364` | `TestParity_CallableSberUnofficialMainMainL336` |
| http_primitives | `load_netscape_cookies`<br>`def load_netscape_cookies(path: Path, *, require_private: bool=True) -> CookieJar` | offline_or_local | `sber_unofficial/_http.py:310–356` | `TestParity_CallableSberUnofficialHttpLoadNetscapeCookiesL310` |
| http_primitives | `inspect_har`<br>`def inspect_har(path: Path) -> tuple[HarInspection, CookieJar]` | offline_or_local | `sber_unofficial/_http.py:460–553` | `TestParity_CallableSberUnofficialHttpInspectHarL460` |
| http_primitives | `seed_from_files`<br>`def seed_from_files(path: Path, cookies_path: Path \| None=None, *, require_private_cookies: bool=True) -> SessionSeed` | offline_or_local | `sber_unofficial/_http.py:556–591` | `TestParity_CallableSberUnofficialHttpSeedFromFilesL556` |
| http_primitives | `seed_from_har`<br>`def seed_from_har(path: Path) -> SessionSeed` | offline_or_local | `sber_unofficial/_http.py:594–595` | `TestParity_CallableSberUnofficialHttpSeedFromHarL594` |
| browser_bootstrap | `is_browser_check`<br>`def is_browser_check(html: str) -> bool` | pure_validation | `sber_unofficial/bootstrap.py:49–57` | `TestParity_CallableSberUnofficialBootstrapIsBrowserCheckL49` |
| client_lifecycle | `AsyncSberClient.from_files`<br>`def from_files(cls, har: Path, cookies: Path \| None=None, *, require_private_har: bool=True, require_private_cookies: bool=True, ca_bundle: str \| Path \| None=None) -> 'AsyncSberClient'` | offline_or_local | `sber_unofficial/client.py:97–113` | `TestParity_CallableSberUnofficialClientAsyncSberClientFromFilesL97` |
| client_lifecycle | `AsyncSberClient.from_session_file`<br>`def from_session_file(cls, path: Path, *, require_private: bool=True, ca_bundle: str \| Path \| None=None) -> 'AsyncSberClient'` | offline_or_local | `sber_unofficial/client.py:116–124` | `TestParity_CallableSberUnofficialClientAsyncSberClientFromSessionFileL116` |
| client_lifecycle | `AsyncSberClient.from_pin_profile`<br>`async def from_pin_profile(cls, path: Path, *, pin: str \| Callable[[], str], require_private: bool=True, browser_bootstrap: BrowserBootstrapProvider \| None=None, browser_bootstrap_timeout: float=30, ca_bundle: str \| Path \| None=None) -> 'AsyncSberClient'` | offline_or_local | `sber_unofficial/client.py:127–152` | `TestParity_CallableSberUnofficialClientAsyncSberClientFromPinProfileL127` |
| client_lifecycle | `AsyncSberClient.from_credentials`<br>`def from_credentials(cls, credentials: SberCredentials, *, session_path: Path \| None=None, browser: BrowserProfile \| None=None, antifraud_deviceprint: str \| None=None, api_base: str \| None=None, web_base: str \| None=None, ca_bundle: str \| Path \| None=None) -> 'AsyncSberClient'` | offline_or_local | `sber_unofficial/client.py:155–181` | `TestParity_CallableSberUnofficialClientAsyncSberClientFromCredentialsL155` |
| client_lifecycle | `AsyncSberClient.aclose`<br>`async def aclose(self) -> None` | offline_or_local | `sber_unofficial/client.py:189–195` | `TestParity_CallableSberUnofficialClientAsyncSberClientAcloseL189` |
| client_lifecycle | `AsyncSberClient.warm_up`<br>`async def warm_up(self, *, force: bool=True) -> None` | session_keepalive | `sber_unofficial/client.py:213–228` | `TestParity_CallableSberUnofficialClientAsyncSberClientWarmUpL213` |
| client_lifecycle | `AsyncSberClient.portfolio`<br>`async def portfolio(self, *, force_update: bool=False) -> BankPortfolio` | read | `sber_unofficial/client.py:230–237` | `TestParity_CallableSberUnofficialClientAsyncSberClientPortfolioL230` |
| client_lifecycle | `AsyncSberClient.export_session`<br>`def export_session(self) -> SessionBundle` | offline_or_local | `sber_unofficial/client.py:474–476` | `TestParity_CallableSberUnofficialClientAsyncSberClientExportSessionL474` |
| client_lifecycle | `AsyncSberClient.export_credentials`<br>`def export_credentials(self) -> SberCredentials` | offline_or_local | `sber_unofficial/client.py:478–480` | `TestParity_CallableSberUnofficialClientAsyncSberClientExportCredentialsL478` |
| deviceprint | `generate_deviceprint`<br>`def generate_deviceprint() -> str` | offline_or_local | `sber_unofficial/deviceprint.py:26–46` | `TestParity_CallableSberUnofficialDeviceprintGenerateDeviceprintL26` |
| deviceprint | `generate_antifraud_deviceprint`<br>`def generate_antifraud_deviceprint(deviceprint: str \| None=None) -> str` | offline_or_local | `sber_unofficial/deviceprint.py:49–56` | `TestParity_CallableSberUnofficialDeviceprintGenerateAntifraudDeviceprintL49` |
| entities | `_OperationsAPI.list`<br>`async def list(self, *, resource: str \| None=None, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> list[Operation]` | offline_or_local | `sber_unofficial/entities.py:22–30` | `TestParity_CallableSberUnofficialEntitiesOperationsAPIListL22` |
| entities | `_OperationsAPI.iter`<br>`def iter(self, *, resource: str \| None=None, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> AsyncIterator[Operation]` | offline_or_local | `sber_unofficial/entities.py:32–40` | `TestParity_CallableSberUnofficialEntitiesOperationsAPIIterL32` |
| entities | `_CardsAPI.rename`<br>`async def rename(self, card_id: str \| int, name: str) -> None` | offline_or_local | `sber_unofficial/entities.py:44–44` | `TestParity_CallableSberUnofficialEntitiesCardsAPIRenameL44` |
| entities | `_TransfersAPI.start`<br>`async def start(self) -> TransferDraft` | offline_or_local | `sber_unofficial/entities.py:48–48` | `TestParity_CallableSberUnofficialEntitiesTransfersAPIStartL48` |
| entities | `_TransfersAPI.prepare`<br>`async def prepare(self, draft: TransferDraft, *, source_id: str, destination_id: str, amount: Decimal, currency: str='RUB', payment_purpose: str='') -> PreparedTransfer` | offline_or_local | `sber_unofficial/entities.py:50–59` | `TestParity_CallableSberUnofficialEntitiesTransfersAPIPrepareL50` |
| entities | `_ProductMethods.operation_resource`<br>`def operation_resource(self) -> str` | offline_or_local | `sber_unofficial/entities.py:74–75` | `TestParity_CallableSberUnofficialEntitiesProductMethodsOperationResourceL74` |
| entities | `_ProductMethods.transfer_resource`<br>`def transfer_resource(self) -> str` | offline_or_local | `sber_unofficial/entities.py:78–79` | `TestParity_CallableSberUnofficialEntitiesProductMethodsTransferResourceL78` |
| entities | `_ProductMethods.operations`<br>`async def operations(self, *, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> list[Operation]` | offline_or_local | `sber_unofficial/entities.py:81–95` | `TestParity_CallableSberUnofficialEntitiesProductMethodsOperationsL81` |
| entities | `_ProductMethods.iter_operations`<br>`def iter_operations(self, *, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> AsyncIterator[Operation]` | offline_or_local | `sber_unofficial/entities.py:97–111` | `TestParity_CallableSberUnofficialEntitiesProductMethodsIterOperationsL97` |
| entities | `_ProductMethods.transfer_to`<br>`async def transfer_to(self, destination: BankCard \| BankAccount, amount: Decimal, *, currency: str='RUB', payment_purpose: str='') -> PreparedTransfer` | financial_workflow_mutation | `sber_unofficial/entities.py:113–135` | `TestParity_CallableSberUnofficialEntitiesProductMethodsTransferToL113` |
| entities | `BankAccount.operation_resource`<br>`def operation_resource(self) -> str` | offline_or_local | `sber_unofficial/entities.py:153–158` | `TestParity_CallableSberUnofficialEntitiesBankAccountOperationResourceL153` |
| entities | `BankAccount.transfer_resource`<br>`def transfer_resource(self) -> str` | offline_or_local | `sber_unofficial/entities.py:161–166` | `TestParity_CallableSberUnofficialEntitiesBankAccountTransferResourceL161` |
| entities | `BankAccount.cards`<br>`def cards(self) -> tuple[BankCard, ...]` | offline_or_local | `sber_unofficial/entities.py:169–170` | `TestParity_CallableSberUnofficialEntitiesBankAccountCardsL169` |
| entities | `BankCard.operation_resource`<br>`def operation_resource(self) -> str` | offline_or_local | `sber_unofficial/entities.py:188–189` | `TestParity_CallableSberUnofficialEntitiesBankCardOperationResourceL188` |
| entities | `BankCard.transfer_resource`<br>`def transfer_resource(self) -> str` | offline_or_local | `sber_unofficial/entities.py:192–193` | `TestParity_CallableSberUnofficialEntitiesBankCardTransferResourceL192` |
| entities | `BankCard.account`<br>`def account(self) -> BankAccount \| None` | offline_or_local | `sber_unofficial/entities.py:196–202` | `TestParity_CallableSberUnofficialEntitiesBankCardAccountL196` |
| entities | `BankCard.rename`<br>`async def rename(self, name: str) -> None` | nonfinancial_bank_mutation | `sber_unofficial/entities.py:204–205` | `TestParity_CallableSberUnofficialEntitiesBankCardRenameL204` |
| entities | `BankPortfolio.raw`<br>`def raw(self) -> Products` | offline_or_local | `sber_unofficial/entities.py:252–253` | `TestParity_CallableSberUnofficialEntitiesBankPortfolioRawL252` |
| entities | `BankPortfolio.accounts`<br>`def accounts(self) -> tuple[BankAccount, ...]` | offline_or_local | `sber_unofficial/entities.py:256–257` | `TestParity_CallableSberUnofficialEntitiesBankPortfolioAccountsL256` |
| entities | `BankPortfolio.cards`<br>`def cards(self) -> tuple[BankCard, ...]` | offline_or_local | `sber_unofficial/entities.py:260–261` | `TestParity_CallableSberUnofficialEntitiesBankPortfolioCardsL260` |
| offline_har | `parse_har`<br>`def parse_har(har: Mapping[str, Any]) -> dict[str, Any]` | offline_or_local | `sber_unofficial/har.py:113–177` | `TestParity_CallableSberUnofficialHarParseHarL113` |
| offline_har | `self_test`<br>`def self_test() -> None` | offline_or_local | `sber_unofficial/har.py:180–254` | `TestParity_CallableSberUnofficialHarSelfTestL180` |
| offline_har | `main`<br>`def main() -> None` | offline_or_local | `sber_unofficial/har.py:257–272` | `TestParity_CallableSberUnofficialHarMainL257` |
| models_and_parsers | `operation_sort_key`<br>`def operation_sort_key(date: str) -> datetime` | offline_or_local | `sber_unofficial/models.py:184–191` | `TestParity_CallableSberUnofficialModelsOperationSortKeyL184` |
| models_and_parsers | `parse_products`<br>`def parse_products(payload: Mapping[str, Any]) -> tuple[list[Account], list[Card]]` | offline_or_local | `sber_unofficial/models.py:215–278` | `TestParity_CallableSberUnofficialModelsParseProductsL215` |
| models_and_parsers | `parse_operations`<br>`def parse_operations(payload: Mapping[str, Any], scopes: Iterable[str]=()) -> list[Operation]` | offline_or_local | `sber_unofficial/models.py:281–344` | `TestParity_CallableSberUnofficialModelsParseOperationsL281` |
| models_and_parsers | `parse_operation_details`<br>`def parse_operation_details(payload: Mapping[str, Any]) -> OperationDetail` | offline_or_local | `sber_unofficial/models.py:478–502` | `TestParity_CallableSberUnofficialModelsParseOperationDetailsL478` |
| models_and_parsers | `parse_card_info`<br>`def parse_card_info(payload: Mapping[str, Any]) -> list[CardInfo]` | offline_or_local | `sber_unofficial/models.py:549–571` | `TestParity_CallableSberUnofficialModelsParseCardInfoL549` |
| models_and_parsers | `parse_pfm_amounts`<br>`def parse_pfm_amounts(payload: Mapping[str, Any]) -> PfmAmounts` | offline_or_local | `sber_unofficial/models.py:601–627` | `TestParity_CallableSberUnofficialModelsParsePfmAmountsL601` |
| auth_crypto | `PinSrp.public_a_hex`<br>`def public_a_hex(self) -> str` | pure_crypto_or_validation | `sber_unofficial/pin.py:85–86` | `TestParity_CallableSberUnofficialPinPinSrpPublicAHexL85` |
| auth_crypto | `PinSrp.process_challenge`<br>`def process_challenge(self, pin: str, *, salt_hex: str, server_b_hex: str) -> str` | pure_crypto_or_validation | `sber_unofficial/pin.py:88–120` | `TestParity_CallableSberUnofficialPinPinSrpProcessChallengeL88` |
| auth_crypto | `PinSrp.verify_server_proof`<br>`def verify_server_proof(self, proof_hex: str) -> bool` | pure_crypto_or_validation | `sber_unofficial/pin.py:122–132` | `TestParity_CallableSberUnofficialPinPinSrpVerifyServerProofL122` |
| auth_crypto | `AsyncPinAuth.from_browser_cookies`<br>`def from_browser_cookies(cls, path: Path, *, browser: BrowserProfile \| None=None, require_private: bool=True, deviceprint: str \| None=None, antifraud_deviceprint: str \| None=None, is_pwa: bool=False, browser_bootstrap: BrowserBootstrapProvider \| None=None, browser_bootstrap_timeout: float=30, ca_bundle: str \| Path \| None=None) -> 'AsyncPinAuth'` | auth_lifecycle_or_bootstrap | `sber_unofficial/pin.py:182–208` | `TestParity_CallableSberUnofficialPinAsyncPinAuthFromBrowserCookiesL182` |
| auth_crypto | `AsyncPinAuth.aclose`<br>`async def aclose(self) -> None` | auth_lifecycle_or_bootstrap | `sber_unofficial/pin.py:216–227` | `TestParity_CallableSberUnofficialPinAsyncPinAuthAcloseL216` |
| auth_crypto | `AsyncPinAuth.login`<br>`async def login(self, pin: str, *, captcha_code: str \| None=None, audio_captcha_code: str \| None=None) -> SessionBundle` | auth_session_mutation | `sber_unofficial/pin.py:229–306` | `TestParity_CallableSberUnofficialPinAsyncPinAuthLoginL229` |
| auth_crypto | `AsyncPinAuth.confirm_otp`<br>`async def confirm_otp(self, code: str) -> SessionBundle` | auth_session_mutation | `sber_unofficial/pin.py:308–322` | `TestParity_CallableSberUnofficialPinAsyncPinAuthConfirmOtpL308` |
| auth_crypto | `AsyncPinAuth.retry_otp`<br>`async def retry_otp(self) -> PinOtpRequired` | auth_session_mutation | `sber_unofficial/pin.py:324–337` | `TestParity_CallableSberUnofficialPinAsyncPinAuthRetryOtpL324` |
| auth_crypto | `AsyncPinAuth.fetch_captcha`<br>`async def fetch_captcha(self, url: str) -> bytes` | auth_lifecycle_or_bootstrap | `sber_unofficial/pin.py:339–364` | `TestParity_CallableSberUnofficialPinAsyncPinAuthFetchCaptchaL339` |
| auth_crypto | `AsyncPinAuth.export_session`<br>`def export_session(self) -> SessionBundle` | auth_lifecycle_or_bootstrap | `sber_unofficial/pin.py:366–369` | `TestParity_CallableSberUnofficialPinAsyncPinAuthExportSessionL366` |
| auth_crypto | `AsyncPrimaryAuth.new`<br>`def new(cls, *, deviceprint: str, browser: BrowserProfile \| None=None, antifraud_deviceprint: str \| None=None, transport: AsyncTransport \| None=None, is_pwa: bool=False, browser_bootstrap: BrowserBootstrapProvider \| None=None, browser_bootstrap_timeout: float=30, ca_bundle: str \| Path \| None=None) -> 'AsyncPrimaryAuth'` | auth_lifecycle_or_bootstrap | `sber_unofficial/pin.py:679–701` | `TestParity_CallableSberUnofficialPinAsyncPrimaryAuthNewL679` |
| auth_crypto | `AsyncPrimaryAuth.login`<br>`async def login(self, login: str, password: str, *, store_login: bool=True, public_key_credential_available: bool=False, captcha_code: str \| None=None, audio_captcha_code: str \| None=None) -> SessionBundle \| None` | auth_session_mutation | `sber_unofficial/pin.py:703–794` | `TestParity_CallableSberUnofficialPinAsyncPrimaryAuthLoginL703` |
| auth_crypto | `AsyncPrimaryAuth.confirm_otp`<br>`async def confirm_otp(self, code: str) -> SessionBundle \| None` | auth_session_mutation | `sber_unofficial/pin.py:796–814` | `TestParity_CallableSberUnofficialPinAsyncPrimaryAuthConfirmOtpL796` |
| auth_crypto | `AsyncPrimaryAuth.retry_otp`<br>`async def retry_otp(self) -> PinOtpRequired` | auth_session_mutation | `sber_unofficial/pin.py:816–841` | `TestParity_CallableSberUnofficialPinAsyncPrimaryAuthRetryOtpL816` |
| auth_crypto | `AsyncPrimaryAuth.create_pin`<br>`async def create_pin(self, pin: str) -> SessionBundle` | auth_session_mutation | `sber_unofficial/pin.py:843–877` | `TestParity_CallableSberUnofficialPinAsyncPrimaryAuthCreatePinL843` |
| resources | `_Client.export_session`<br>`def export_session(self) -> SessionBundle` | read | `sber_unofficial/resources.py:81–81` | `TestParity_CallableSberUnofficialResourcesClientExportSessionL81` |
| resources | `_Client.export_credentials`<br>`def export_credentials(self) -> SberCredentials` | read | `sber_unofficial/resources.py:82–82` | `TestParity_CallableSberUnofficialResourcesClientExportCredentialsL82` |
| resources | `_Client.warm_up`<br>`async def warm_up(self, *, force: bool=True) -> None` | session_keepalive | `sber_unofficial/resources.py:83–83` | `TestParity_CallableSberUnofficialResourcesClientWarmUpL83` |
| resources | `_Client.portfolio`<br>`async def portfolio(self, *, force_update: bool=False) -> BankPortfolio` | read | `sber_unofficial/resources.py:84–84` | `TestParity_CallableSberUnofficialResourcesClientPortfolioL84` |
| resources | `ProductsAPI.get`<br>`async def get(self, *, force_update: bool=False) -> Products` | read | `sber_unofficial/resources.py:91–97` | `TestParity_CallableSberUnofficialResourcesProductsAPIGetL91` |
| resources | `AccountsAPI.list`<br>`async def list(self, *, force_update: bool=False) -> tuple[BankAccount, ...]` | read | `sber_unofficial/resources.py:104–105` | `TestParity_CallableSberUnofficialResourcesAccountsAPIListL104` |
| resources | `OperationsAPI.page`<br>`async def page(self, *, resource: str \| None=None, offset: int=0, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None) -> OperationsPage` | read | `sber_unofficial/resources.py:112–154` | `TestParity_CallableSberUnofficialResourcesOperationsAPIPageL112` |
| resources | `OperationsAPI.iter`<br>`async def iter(self, *, resource: str \| None=None, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> AsyncIterator[Operation]` | read | `sber_unofficial/resources.py:156–186` | `TestParity_CallableSberUnofficialResourcesOperationsAPIIterL156` |
| resources | `OperationsAPI.list`<br>`async def list(self, *, resource: str \| None=None, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> list[Operation]` | read | `sber_unofficial/resources.py:188–207` | `TestParity_CallableSberUnofficialResourcesOperationsAPIListL188` |
| resources | `OperationsAPI.details`<br>`async def details(self, uoh_id: str) -> OperationDetail` | read | `sber_unofficial/resources.py:209–214` | `TestParity_CallableSberUnofficialResourcesOperationsAPIDetailsL209` |
| resources | `CardsAPI.list`<br>`async def list(self, *, force_update: bool=False) -> tuple[BankCard, ...]` | read | `sber_unofficial/resources.py:221–222` | `TestParity_CallableSberUnofficialResourcesCardsAPIListL221` |
| resources | `CardsAPI.rename`<br>`async def rename(self, card_id: str \| int, name: str) -> None` | nonfinancial_bank_mutation | `sber_unofficial/resources.py:224–243` | `TestParity_CallableSberUnofficialResourcesCardsAPIRenameL224` |
| resources | `CardsAPI.info`<br>`async def info(self, *card_ids: str \| int) -> tuple[CardInfo, ...]` | read | `sber_unofficial/resources.py:245–251` | `TestParity_CallableSberUnofficialResourcesCardsAPIInfoL245` |
| resources | `CardsAPI.limits`<br>`async def limits(self, card_id: str \| int) -> CardLimits \| None` | read | `sber_unofficial/resources.py:253–256` | `TestParity_CallableSberUnofficialResourcesCardsAPILimitsL253` |
| resources | `AnalyticsAPI.amounts`<br>`async def amounts(self, *, from_: date \| datetime, to: date \| datetime, income_type: str='outcome', between_own: bool=True, open_banking: bool=False, show_categories: bool=True, show_products: bool=True) -> PfmAmounts` | read | `sber_unofficial/resources.py:265–300` | `TestParity_CallableSberUnofficialResourcesAnalyticsAPIAmountsL265` |
| resources | `TransfersAPI.start`<br>`async def start(self) -> TransferDraft` | financial_workflow_mutation | `sber_unofficial/resources.py:311–334` | `TestParity_CallableSberUnofficialResourcesTransfersAPIStartL311` |
| resources | `TransfersAPI.prepare`<br>`async def prepare(self, draft: TransferDraft, *, source_id: str, destination_id: str, amount: Decimal, currency: str='RUB', payment_purpose: str='') -> PreparedTransfer` | financial_workflow_mutation | `sber_unofficial/resources.py:336–423` | `TestParity_CallableSberUnofficialResourcesTransfersAPIPrepareL336` |
| resources | `TransfersAPI.confirm`<br>`async def confirm(self, transfer: PreparedTransfer) -> TransferResult` | financial_confirmation_mutation | `sber_unofficial/resources.py:425–493` | `TestParity_CallableSberUnofficialResourcesTransfersAPIConfirmL425` |
| resources | `SessionAPI.warm_up`<br>`async def warm_up(self, *, force: bool=True) -> None` | session_keepalive | `sber_unofficial/resources.py:500–501` | `TestParity_CallableSberUnofficialResourcesSessionAPIWarmUpL500` |
| resources | `SessionAPI.export`<br>`def export(self, path: Path \| None=None) -> SessionBundle` | read | `sber_unofficial/resources.py:503–507` | `TestParity_CallableSberUnofficialResourcesSessionAPIExportL503` |
| resources | `SessionAPI.credentials`<br>`def credentials(self) -> SberCredentials` | read | `sber_unofficial/resources.py:509–511` | `TestParity_CallableSberUnofficialResourcesSessionAPICredentialsL509` |
| session_state | `BrowserProfile.from_mapping`<br>`def from_mapping(cls, headers: Mapping[str, str]) -> 'BrowserProfile'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:111–112` | `TestParity_CallableSberUnofficialSessionBrowserProfileFromMappingL111` |
| session_state | `BrowserProfile.from_har`<br>`def from_har(cls, path: Path, *, require_private: bool=True) -> 'BrowserProfile'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:115–123` | `TestParity_CallableSberUnofficialSessionBrowserProfileFromHarL115` |
| session_state | `BrowserProfile.as_dict`<br>`def as_dict(self) -> dict[str, str]` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:125–126` | `TestParity_CallableSberUnofficialSessionBrowserProfileAsDictL125` |
| session_state | `SberCredentials.from_bundle`<br>`def from_bundle(cls, bundle: 'SessionBundle') -> 'SberCredentials'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:154–177` | `TestParity_CallableSberUnofficialSessionSberCredentialsFromBundleL154` |
| session_state | `SberCredentials.to_bundle`<br>`def to_bundle(self, *, browser: BrowserProfile \| None=None, api_base: str=APP_ORIGIN, web_base: str=APP_ORIGIN, deviceprint: str \| None=None, antifraud_deviceprint: str \| None=None) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:179–208` | `TestParity_CallableSberUnofficialSessionSberCredentialsToBundleL179` |
| session_state | `SessionBundle.from_seed`<br>`def from_seed(cls, seed: SessionSeed, *, antifraud_deviceprint: str \| None=None) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:264–278` | `TestParity_CallableSberUnofficialSessionSessionBundleFromSeedL264` |
| session_state | `SessionBundle.from_files`<br>`def from_files(cls, har: Path, cookies: Path \| None=None, *, require_private_har: bool=True, require_private_cookies: bool=True) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:281–299` | `TestParity_CallableSberUnofficialSessionSessionBundleFromFilesL281` |
| session_state | `SessionBundle.from_browser_cookies`<br>`def from_browser_cookies(cls, path: Path, *, browser: BrowserProfile \| None=None, deviceprint: str \| None=None, antifraud_deviceprint: str \| None=None, require_private: bool=True) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:302–377` | `TestParity_CallableSberUnofficialSessionSessionBundleFromBrowserCookiesL302` |
| session_state | `SessionBundle.with_antifraud_from_har`<br>`def with_antifraud_from_har(self, path: Path, *, require_private: bool=True) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:379–394` | `TestParity_CallableSberUnofficialSessionSessionBundleWithAntifraudFromHarL379` |
| session_state | `SessionBundle.load`<br>`def load(cls, path: Path, *, require_private: bool=True) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:397–512` | `TestParity_CallableSberUnofficialSessionSessionBundleLoadL397` |
| session_state | `SessionBundle.save`<br>`def save(self, path: Path) -> None` | local_secret_state_write | `sber_unofficial/session.py:514–516` | `TestParity_CallableSberUnofficialSessionSessionBundleSaveL514` |
| session_state | `SessionBundle.to_seed`<br>`def to_seed(self, *, require_ready: bool=True) -> SessionSeed` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:518–530` | `TestParity_CallableSberUnofficialSessionSessionBundleToSeedL518` |
| session_state | `SessionBundle.with_cookie_jar`<br>`def with_cookie_jar(self, jar: CookieJar) -> 'SessionBundle'` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:532–549` | `TestParity_CallableSberUnofficialSessionSessionBundleWithCookieJarL532` |
| session_state | `SessionBundle.redacted`<br>`def redacted(self) -> dict[str, Any]` | local_secret_state_or_cookie_metadata | `sber_unofficial/session.py:551–576` | `TestParity_CallableSberUnofficialSessionSessionBundleRedactedL551` |
| transport | `ResponseLike.json`<br>`def json(self) -> Any` | cookie_metadata | `sber_unofficial/transport.py:30–30` | `TestParity_CallableSberUnofficialTransportResponseLikeJsonL30` |
| transport | `AsyncTransport.get`<br>`async def get(self, url: str, *, accept_encoding: str, headers: Mapping[str, str \| None] \| None=None) -> ResponseLike` | generic_transport_capability | `sber_unofficial/transport.py:34–40` | `TestParity_CallableSberUnofficialTransportAsyncTransportGetL34` |
| transport | `AsyncTransport.post`<br>`async def post(self, url: str, *, json: Mapping[str, Any] \| None, accept_encoding: str, headers: Mapping[str, str \| None] \| None=None) -> ResponseLike` | generic_transport_capability | `sber_unofficial/transport.py:42–49` | `TestParity_CallableSberUnofficialTransportAsyncTransportPostL42` |
| transport | `AsyncTransport.post_form`<br>`async def post_form(self, url: str, *, data: Mapping[str, str], accept_encoding: str, headers: Mapping[str, str \| None] \| None=None) -> ResponseLike` | generic_transport_capability | `sber_unofficial/transport.py:51–58` | `TestParity_CallableSberUnofficialTransportAsyncTransportPostFormL51` |
| transport | `AsyncTransport.cookie_jar`<br>`def cookie_jar(self) -> CookieJar` | generic_transport_capability | `sber_unofficial/transport.py:60–60` | `TestParity_CallableSberUnofficialTransportAsyncTransportCookieJarL60` |
| transport | `AsyncTransport.aclose`<br>`async def aclose(self) -> None` | generic_transport_capability | `sber_unofficial/transport.py:62–62` | `TestParity_CallableSberUnofficialTransportAsyncTransportAcloseL62` |
| transport | `_ResponseCookieJar.clear`<br>`def clear(self, domain: str \| None=None, path: str \| None=None, name: str \| None=None) -> None` | cookie_metadata | `sber_unofficial/transport.py:76–79` | `TestParity_CallableSberUnofficialTransportResponseCookieJarClearL76` |
| transport | `CurlAsyncTransport.get`<br>`async def get(self, url: str, *, accept_encoding: str, headers: Mapping[str, str \| None] \| None=None) -> ResponseLike` | generic_transport_capability | `sber_unofficial/transport.py:156–178` | `TestParity_CallableSberUnofficialTransportCurlAsyncTransportGetL156` |
| transport | `CurlAsyncTransport.post`<br>`async def post(self, url: str, *, json: Mapping[str, Any] \| None, accept_encoding: str, headers: Mapping[str, str \| None] \| None=None) -> ResponseLike` | generic_transport_capability | `sber_unofficial/transport.py:180–204` | `TestParity_CallableSberUnofficialTransportCurlAsyncTransportPostL180` |
| transport | `CurlAsyncTransport.post_form`<br>`async def post_form(self, url: str, *, data: Mapping[str, str], accept_encoding: str, headers: Mapping[str, str \| None] \| None=None) -> ResponseLike` | generic_transport_capability | `sber_unofficial/transport.py:206–224` | `TestParity_CallableSberUnofficialTransportCurlAsyncTransportPostFormL206` |
| transport | `CurlAsyncTransport.cookie_jar`<br>`def cookie_jar(self) -> CookieJar` | generic_transport_capability | `sber_unofficial/transport.py:226–227` | `TestParity_CallableSberUnofficialTransportCurlAsyncTransportCookieJarL226` |
| transport | `CurlAsyncTransport.aclose`<br>`async def aclose(self) -> None` | generic_transport_capability | `sber_unofficial/transport.py:229–232` | `TestParity_CallableSberUnofficialTransportCurlAsyncTransportAcloseL229` |
| mcp_demo | `_DemoOperations.list`<br>`async def list(self, resource: str \| None=None, limit: int=30, from_: Any=None, to: Any=None, max_pages: int=3) -> list[Operation]` | offline_or_local | `mcp/sber_unofficial_mcp/demo.py:94–98` | `TestParity_CallableMcpSberUnofficialMcpDemoDemoOperationsListL94` |
| mcp_demo | `_DemoOperations.page`<br>`async def page(self, resource: str \| None=None, offset: int=0, limit: int=30, from_: Any=None, to: Any=None) -> OperationsPage` | offline_or_local | `mcp/sber_unofficial_mcp/demo.py:100–105` | `TestParity_CallableMcpSberUnofficialMcpDemoDemoOperationsPageL100` |
| mcp_demo | `DemoClient.portfolio`<br>`async def portfolio(self, force_update: bool=False) -> BankPortfolio` | offline_or_local | `mcp/sber_unofficial_mcp/demo.py:120–121` | `TestParity_CallableMcpSberUnofficialMcpDemoDemoClientPortfolioL120` |
| mcp_demo | `DemoClient.aclose`<br>`async def aclose(self) -> None` | offline_or_local | `mcp/sber_unofficial_mcp/demo.py:123–124` | `TestParity_CallableMcpSberUnofficialMcpDemoDemoClientAcloseL123` |
| mcp_tools | `ok`<br>`def ok(data: dict, *, code: str='ok', session_id: str \| None=None, state: str \| None=None) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:183–186` | `TestParity_CallableMcpSberUnofficialMcpServerOkL183` |
| mcp_tools | `err`<br>`def err(code: str, next_action: str, *, retryable: bool=False, details: dict \| None=None, session_id: str \| None=None, state: str \| None=None) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:189–192` | `TestParity_CallableMcpSberUnofficialMcpServerErrL189` |
| mcp_tools | `challenge`<br>`def challenge(code: str, *, details: dict, session_id: str, state: str, next_action: str=NA_SUBMIT) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:195–198` | `TestParity_CallableMcpSberUnofficialMcpServerChallengeL195` |
| mcp_tools | `map_error`<br>`def map_error(exc: BaseException) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:267–310` | `TestParity_CallableMcpSberUnofficialMcpServerMapErrorL267` |
| mcp_tools | `Config.login_password`<br>`def login_password(self) -> tuple[str, str]` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:407–416` | `TestParity_CallableMcpSberUnofficialMcpServerConfigLoginPasswordL407` |
| mcp_tools | `Config.deviceprint`<br>`def deviceprint(self) -> str` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:418–431` | `TestParity_CallableMcpSberUnofficialMcpServerConfigDeviceprintL418` |
| mcp_tools | `Config.antifraud`<br>`def antifraud(self) -> str \| None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:433–447` | `TestParity_CallableMcpSberUnofficialMcpServerConfigAntifraudL433` |
| mcp_tools | `Config.cookies_export`<br>`def cookies_export(self) -> Path \| None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:449–452` | `TestParity_CallableMcpSberUnofficialMcpServerConfigCookiesExportL449` |
| mcp_tools | `Config.ufs_credentials`<br>`def ufs_credentials(self) -> SberCredentials \| None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:454–465` | `TestParity_CallableMcpSberUnofficialMcpServerConfigUfsCredentialsL454` |
| mcp_tools | `Config.artifact_status`<br>`def artifact_status(self) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:467–481` | `TestParity_CallableMcpSberUnofficialMcpServerConfigArtifactStatusL467` |
| mcp_tools | `Registry.get`<br>`def get(self, session_id: str) -> SessionState` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:536–542` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryGetL536` |
| mcp_tools | `Registry.resolve`<br>`def resolve(self, session_id: str \| None=None, profile: str='default') -> SessionState` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:544–562` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryResolveL544` |
| mcp_tools | `Registry.arm_inbox`<br>`def arm_inbox(self, st: SessionState, expects: list[str]) -> Path` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:580–601` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryArmInboxL580` |
| mcp_tools | `Registry.read_inbox`<br>`def read_inbox(self, st: SessionState) -> list[str] \| None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:603–626` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryReadInboxL603` |
| mcp_tools | `Registry.clear_inbox`<br>`def clear_inbox(self, st: SessionState) -> None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:628–637` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryClearInboxL628` |
| mcp_tools | `Registry.audit`<br>`def audit(self, operation: str, args: dict, outcome: str, document_id: str \| None=None) -> str` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:640–651` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryAuditL640` |
| mcp_tools | `Registry.close`<br>`async def close(self, st: SessionState) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:654–667` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryCloseL654` |
| mcp_tools | `Registry.close_all`<br>`async def close_all(self) -> None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:669–671` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryCloseAllL669` |
| mcp_tools | `Registry.reap_once`<br>`async def reap_once(self) -> None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:673–681` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryReapOnceL673` |
| mcp_tools | `Registry.reap_forever`<br>`async def reap_forever(self) -> None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:683–689` | `TestParity_CallableMcpSberUnofficialMcpServerRegistryReapForeverL683` |
| mcp_tools | `sber_setup_status`<br>`async def sber_setup_status(profile: str \| None=None) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:831–869` | `TestParity_CallableMcpSberUnofficialMcpServerSberSetupStatusL831` |
| mcp_tools | `sber_auth_start`<br>`async def sber_auth_start(mode: str='auto', profile: str='default') -> dict` | auth_session_mutation | `mcp/sber_unofficial_mcp/server.py:898–918` | `TestParity_CallableMcpSberUnofficialMcpServerSberAuthStartL898` |
| mcp_tools | `sber_auth_continue`<br>`async def sber_auth_continue(session_id: str \| None=None, wait_seconds: int=30, use_audio_captcha: bool=False) -> dict` | auth_session_mutation | `mcp/sber_unofficial_mcp/server.py:1054–1085` | `TestParity_CallableMcpSberUnofficialMcpServerSberAuthContinueL1054` |
| mcp_tools | `sber_auth_resend_otp`<br>`async def sber_auth_resend_otp(session_id: str \| None=None) -> dict` | auth_session_mutation | `mcp/sber_unofficial_mcp/server.py:1170–1182` | `TestParity_CallableMcpSberUnofficialMcpServerSberAuthResendOtpL1170` |
| mcp_tools | `sber_session_info`<br>`async def sber_session_info(session_id: str \| None=None, check_live: bool=False) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:1185–1208` | `TestParity_CallableMcpSberUnofficialMcpServerSberSessionInfoL1185` |
| mcp_tools | `sber_session_close`<br>`async def sber_session_close(session_id: str \| None=None) -> dict` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:1211–1219` | `TestParity_CallableMcpSberUnofficialMcpServerSberSessionCloseL1211` |
| mcp_tools | `sber_products`<br>`async def sber_products(session_id: str \| None=None, force_update: bool=False) -> dict` | read | `mcp/sber_unofficial_mcp/server.py:1228–1243` | `TestParity_CallableMcpSberUnofficialMcpServerSberProductsL1228` |
| mcp_tools | `sber_operations`<br>`async def sber_operations(session_id: str \| None=None, resource: str \| None=None, from_date: str \| None=None, to_date: str \| None=None, limit: int=30, max_pages: int=3) -> dict` | read | `mcp/sber_unofficial_mcp/server.py:1252–1275` | `TestParity_CallableMcpSberUnofficialMcpServerSberOperationsL1252` |
| mcp_tools | `sber_operations_page`<br>`async def sber_operations_page(session_id: str \| None=None, resource: str \| None=None, offset: int=0, limit: int=30, from_date: str \| None=None, to_date: str \| None=None) -> dict` | read | `mcp/sber_unofficial_mcp/server.py:1278–1296` | `TestParity_CallableMcpSberUnofficialMcpServerSberOperationsPageL1278` |
| mcp_tools | `sber_card_rename`<br>`async def sber_card_rename(card_id: str, name: str, session_id: str \| None=None) -> dict` | nonfinancial_bank_mutation | `mcp/sber_unofficial_mcp/server.py:1299–1321` | `TestParity_CallableMcpSberUnofficialMcpServerSberCardRenameL1299` |
| mcp_tools | `sber_transfer_start`<br>`async def sber_transfer_start(session_id: str \| None=None) -> dict` | financial_workflow_mutation | `mcp/sber_unofficial_mcp/server.py:1324–1344` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferStartL1324` |
| mcp_tools | `sber_transfer_prepare`<br>`async def sber_transfer_prepare(draft_id: str, source_id: str, destination_id: str, amount: str, currency: str='RUB', payment_purpose: str='', session_id: str \| None=None) -> dict` | financial_workflow_mutation | `mcp/sber_unofficial_mcp/server.py:1347–1396` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferPrepareL1347` |
| mcp_tools | `sber_transfer_confirm`<br>`async def sber_transfer_confirm(confirmation_token: str, acknowledged_amount: str, acknowledged_destination_id: str, session_id: str \| None=None) -> dict` | financial_confirmation_mutation | `mcp/sber_unofficial_mcp/server.py:1399–1461` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferConfirmL1399` |
| mcp_tools | `sber_transfer_resolve_uncertain`<br>`async def sber_transfer_resolve_uncertain(audit_id: str, found: bool, session_id: str \| None=None) -> dict` | financial_control_reconciliation | `mcp/sber_unofficial_mcp/server.py:1464–1478` | `TestParity_CallableMcpSberUnofficialMcpServerSberTransferResolveUncertainL1464` |
| mcp_tools | `main`<br>`def main() -> None` | mcp_control_or_local_state | `mcp/sber_unofficial_mcp/server.py:1987–2010` | `TestParity_CallableMcpSberUnofficialMcpServerMainL1987` |
| legacy_sync | `ResponseLike.json`<br>`def json(self) -> Any` | read | `sber_client.py:132–132` | `TestParity_CallableSberClientResponseLikeJsonL132` |
| legacy_sync | `SessionLike.post`<br>`def post(self, url: str, **kwargs: Any) -> ResponseLike` | read | `sber_client.py:136–136` | `TestParity_CallableSberClientSessionLikePostL136` |
| legacy_sync | `SessionLike.close`<br>`def close(self) -> None` | read | `sber_client.py:137–137` | `TestParity_CallableSberClientSessionLikeCloseL137` |
| legacy_sync | `SberClient.from_har`<br>`def from_har(cls, path: Path, cookies: Path \| None=None) -> 'SberClient'` | read | `sber_client.py:179–180` | `TestParity_CallableSberClientSberClientFromHarL179` |
| legacy_sync | `SberClient.close`<br>`def close(self) -> None` | read | `sber_client.py:188–189` | `TestParity_CallableSberClientSberClientCloseL188` |
| legacy_sync | `SberClient.warm_up`<br>`def warm_up(self) -> None` | read | `sber_client.py:225–227` | `TestParity_CallableSberClientSberClientWarmUpL225` |
| legacy_sync | `SberClient.products`<br>`def products(self, *, force_update: bool=False) -> tuple[list[Account], list[Card]]` | read | `sber_client.py:229–235` | `TestParity_CallableSberClientSberClientProductsL229` |
| legacy_sync | `SberClient.operations_page`<br>`def operations_page(self, *, resource: str \| None=None, offset: int=0, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None) -> OperationsPage` | read | `sber_client.py:237–280` | `TestParity_CallableSberClientSberClientOperationsPageL237` |
| legacy_sync | `SberClient.operations`<br>`def operations(self, *, resource: str \| None=None, limit: int=30, from_: date \| datetime \| None=None, to: date \| datetime \| None=None, max_pages: int=100) -> list[Operation]` | read | `sber_client.py:282–309` | `TestParity_CallableSberClientSberClientOperationsL282` |
| legacy_sync | `main`<br>`def main() -> None` | cli_auth_or_read_dispatch | `sber_client.py:329–376` | `TestParity_CallableSberClientMainL329` |
| safe_launcher | `private_directory`<br>`def private_directory(path)` | local_filesystem_control | `../local_sber.py:22–30` | `TestParity_CallableLocalSberPrivateDirectoryL22` |
| safe_launcher | `make_auth`<br>`def make_auth(ca_bundle)` | local_auth_enrollment | `../local_sber.py:33–50` | `TestParity_CallableLocalSberMakeAuthL33` |
| safe_launcher | `hidden_prompt`<br>`def hidden_prompt(prompt, label)` | owner_terminal_or_offline_status | `../local_sber.py:53–60` | `TestParity_CallableLocalSberHiddenPromptL53` |
| safe_launcher | `authorize`<br>`async def authorize(auth, profile, *, prompt)` | local_auth_enrollment | `../local_sber.py:88–121` | `TestParity_CallableLocalSberAuthorizeL88` |
| safe_launcher | `main`<br>`def main(argv=None)` | owner_terminal_or_offline_status | `../local_sber.py:124–162` | `TestParity_CallableLocalSberMainL124` |

### Inherited APIs, explicitly retained

| Public alias | Defining contract | Source | Required Go test |
|---|---|---|---|
| `BankAccount.operations` | `callable:sber_unofficial/entities.py:_ProductMethods.operations` | `sber_unofficial/entities.py:81–95` | `TestParity_InheritedSberUnofficialEntitiesBankAccountOperations` |
| `BankAccount.iter_operations` | `callable:sber_unofficial/entities.py:_ProductMethods.iter_operations` | `sber_unofficial/entities.py:97–111` | `TestParity_InheritedSberUnofficialEntitiesBankAccountIterOperations` |
| `BankAccount.transfer_to` | `callable:sber_unofficial/entities.py:_ProductMethods.transfer_to` | `sber_unofficial/entities.py:113–135` | `TestParity_InheritedSberUnofficialEntitiesBankAccountTransferTo` |
| `BankCard.operations` | `callable:sber_unofficial/entities.py:_ProductMethods.operations` | `sber_unofficial/entities.py:81–95` | `TestParity_InheritedSberUnofficialEntitiesBankCardOperations` |
| `BankCard.iter_operations` | `callable:sber_unofficial/entities.py:_ProductMethods.iter_operations` | `sber_unofficial/entities.py:97–111` | `TestParity_InheritedSberUnofficialEntitiesBankCardIterOperations` |
| `BankCard.transfer_to` | `callable:sber_unofficial/entities.py:_ProductMethods.transfer_to` | `sber_unofficial/entities.py:113–135` | `TestParity_InheritedSberUnofficialEntitiesBankCardTransferTo` |
| `AsyncPrimaryAuth.from_browser_cookies` | `callable:sber_unofficial/pin.py:AsyncPinAuth.from_browser_cookies` | `sber_unofficial/pin.py:182–208` | `TestParity_InheritedSberUnofficialPinAsyncPrimaryAuthFromBrowserCookies` |
| `AsyncPrimaryAuth.__aenter__` | `callable:sber_unofficial/pin.py:AsyncPinAuth.__aenter__` | `sber_unofficial/pin.py:210–211` | `TestParity_InheritedSberUnofficialPinAsyncPrimaryAuthAenter` |
| `AsyncPrimaryAuth.__aexit__` | `callable:sber_unofficial/pin.py:AsyncPinAuth.__aexit__` | `sber_unofficial/pin.py:213–214` | `TestParity_InheritedSberUnofficialPinAsyncPrimaryAuthAexit` |
| `AsyncPrimaryAuth.aclose` | `callable:sber_unofficial/pin.py:AsyncPinAuth.aclose` | `sber_unofficial/pin.py:216–227` | `TestParity_InheritedSberUnofficialPinAsyncPrimaryAuthAclose` |
| `AsyncPrimaryAuth.fetch_captcha` | `callable:sber_unofficial/pin.py:AsyncPinAuth.fetch_captcha` | `sber_unofficial/pin.py:339–364` | `TestParity_InheritedSberUnofficialPinAsyncPrimaryAuthFetchCaptcha` |
| `AsyncPrimaryAuth.export_session` | `callable:sber_unofficial/pin.py:AsyncPinAuth.export_session` | `sber_unofficial/pin.py:366–369` | `TestParity_InheritedSberUnofficialPinAsyncPrimaryAuthExportSession` |

## Public models, error types and protocol/type declarations

Field order, exact annotations/default expressions, dataclass frozen/slots/kw-only/repr flags, bases and public constructor attributes are preserved per class in JSON. BankAccount/BankCard inherit Account/Card fields; no reflection-driven helper or full-PAN field is added. Declared private types are included because their state semantics cross public boundaries.

| Type | Fields (name: type; default if any) | Source | Required Go test |
|---|---|---|---|
| `HarInspection` | api_base: str \| None; web_base: str \| None; cookie_count: int; has_sensitive_session: bool; target_request_count: int; observed_headers: dict[str, str] = field(repr=False); antifraud_deviceprint: str \| None = field(default=None, repr=False) | `sber_unofficial/_http.py:85–92` | `TestParity_TypeSberUnofficialHttpHarInspectionL85` |
| `SessionSeed` | api_base: str; web_base: str; cookies: CookieJar = field(repr=False); observed_headers: dict[str, str] = field(repr=False) | `sber_unofficial/_http.py:96–100` | `TestParity_TypeSberUnofficialHttpSessionSeedL96` |
| `BrowserBootstrapResult` | html: str; cookies: tuple[CookieRecord, ...]; browser: BrowserProfile; url: str | `sber_unofficial/bootstrap.py:22–40` | `TestParity_TypeSberUnofficialBootstrapBrowserBootstrapResultL22` |
| `BrowserBootstrapProvider` | See base types, constructors and callable contracts | `sber_unofficial/bootstrap.py:43–46` | `TestParity_TypeSberUnofficialBootstrapBrowserBootstrapProviderL43` |
| `FirefoxBrowserBootstrap` | See base types, constructors and callable contracts | `sber_unofficial/bootstrap.py:60–192` | `TestParity_TypeSberUnofficialBootstrapFirefoxBrowserBootstrapL60` |
| `AsyncSberClient` | See base types, constructors and callable contracts | `sber_unofficial/client.py:52–480` | `TestParity_TypeSberUnofficialClientAsyncSberClientL52` |
| `_OperationsAPI` | See base types, constructors and callable contracts | `sber_unofficial/entities.py:21–40` | `TestParity_TypeSberUnofficialEntitiesOperationsAPIL21` |
| `_CardsAPI` | See base types, constructors and callable contracts | `sber_unofficial/entities.py:43–44` | `TestParity_TypeSberUnofficialEntitiesCardsAPIL43` |
| `_TransfersAPI` | See base types, constructors and callable contracts | `sber_unofficial/entities.py:47–59` | `TestParity_TypeSberUnofficialEntitiesTransfersAPIL47` |
| `_Client` | operations: _OperationsAPI; cards: _CardsAPI; transfers: _TransfersAPI | `sber_unofficial/entities.py:62–65` | `TestParity_TypeSberUnofficialEntitiesClientL62` |
| `_ProductMethods` | _client: _Client | `sber_unofficial/entities.py:68–135` | `TestParity_TypeSberUnofficialEntitiesProductMethodsL68` |
| `BankAccount` | _portfolio: 'BankPortfolio' | `sber_unofficial/entities.py:138–170` | `TestParity_TypeSberUnofficialEntitiesBankAccountL138` |
| `BankCard` | _portfolio: 'BankPortfolio' | `sber_unofficial/entities.py:173–205` | `TestParity_TypeSberUnofficialEntitiesBankCardL173` |
| `BankPortfolio` | See base types, constructors and callable contracts | `sber_unofficial/entities.py:208–264` | `TestParity_TypeSberUnofficialEntitiesBankPortfolioL208` |
| `SberError` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:4–5` | `TestParity_TypeSberUnofficialErrorsSberErrorL4` |
| `MissingSession` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:8–9` | `TestParity_TypeSberUnofficialErrorsMissingSessionL8` |
| `InsecureSessionFile` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:12–13` | `TestParity_TypeSberUnofficialErrorsInsecureSessionFileL12` |
| `AuthenticationExpired` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:16–17` | `TestParity_TypeSberUnofficialErrorsAuthenticationExpiredL16` |
| `ApiError` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:20–21` | `TestParity_TypeSberUnofficialErrorsApiErrorL20` |
| `ApiRejected` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:24–42` | `TestParity_TypeSberUnofficialErrorsApiRejectedL24` |
| `TransportError` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:45–46` | `TestParity_TypeSberUnofficialErrorsTransportErrorL45` |
| `MutationUncertain` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:49–50` | `TestParity_TypeSberUnofficialErrorsMutationUncertainL49` |
| `PinAuthError` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:53–69` | `TestParity_TypeSberUnofficialErrorsPinAuthErrorL53` |
| `PinCaptchaRequired` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:72–86` | `TestParity_TypeSberUnofficialErrorsPinCaptchaRequiredL72` |
| `PinOtpRequired` | See base types, constructors and callable contracts | `sber_unofficial/errors.py:89–98` | `TestParity_TypeSberUnofficialErrorsPinOtpRequiredL89` |
| `ParseError` | See base types, constructors and callable contracts | `sber_unofficial/models.py:26–27` | `TestParity_TypeSberUnofficialModelsParseErrorL26` |
| `Money` | amount: Decimal; currency: str | `sber_unofficial/models.py:31–33` | `TestParity_TypeSberUnofficialModelsMoneyL31` |
| `Account` | id: str; name: str; last4: str; state: str; hidden: bool; arrested: bool; balance: Money \| None; kind: str = 'ctaccount' | `sber_unofficial/models.py:37–45` | `TestParity_TypeSberUnofficialModelsAccountL37` |
| `Card` | id: str; name: str; last4: str; type: str; state: str; hidden: bool; arrested: bool; is_main: bool; balance: Money \| None; balance_source: str \| None; account_id: str \| None; account_balance: Money \| None | `sber_unofficial/models.py:49–61` | `TestParity_TypeSberUnofficialModelsCardL49` |
| `Resource` | type: str; id: str | `sber_unofficial/models.py:65–67` | `TestParity_TypeSberUnofficialModelsResourceL65` |
| `Operation` | id: str; date: str; form: str; type: str; classification_code: str; creation_channel: str; state: str; state_name: str; state_description: str; merchant: str; description: str; amount: Money \| None; billing_amount: Money \| None; national_amount: Money \| None; commission: Money \| None; tips: Money \| None; refusal_reason: str; from_resource: Resource \| None; to_resource: Resource \| None; scope_card_ids: tuple[str, ...]; is_financial: bool \| None; is_hidden: bool; balance_after: Money \| None = None; balance_after_resource_id: str \| None = None; balance_after_resource_name: str \| None = None | `sber_unofficial/models.py:71–96` | `TestParity_TypeSberUnofficialModelsOperationL71` |
| `CardLedgerEntry` | operation_id: str; card_id: str; direction: str; amount: Money \| None | `sber_unofficial/models.py:100–104` | `TestParity_TypeSberUnofficialModelsCardLedgerEntryL100` |
| `Products` | accounts: tuple[Account, ...]; cards: tuple[Card, ...] | `sber_unofficial/models.py:381–383` | `TestParity_TypeSberUnofficialModelsProductsL381` |
| `OperationsPage` | operations: tuple[Operation, ...]; next_offset: int \| None | `sber_unofficial/models.py:387–389` | `TestParity_TypeSberUnofficialModelsOperationsPageL387` |
| `TransferResource` | id: str; kind: str; name: str; currency: str | `sber_unofficial/models.py:393–400` | `TestParity_TypeSberUnofficialModelsTransferResourceL393` |
| `TransferDraft` | pid: str; flow: str; state: str; sources: tuple[TransferResource, ...]; destinations: tuple[TransferResource, ...] | `sber_unofficial/models.py:404–415` | `TestParity_TypeSberUnofficialModelsTransferDraftL404` |
| `PreparedTransfer` | pid: str; flow: str; state: str; source_id: str; destination_id: str; amount: Money; payment_purpose: str | `sber_unofficial/models.py:419–432` | `TestParity_TypeSberUnofficialModelsPreparedTransferL419` |
| `TransferResult` | pid: str; flow: str; state: str; document_id: str \| None | `sber_unofficial/models.py:436–446` | `TestParity_TypeSberUnofficialModelsTransferResultL436` |
| `OperationDetailField` | name: str; type: str; value: Money \| str \| None | `sber_unofficial/models.py:459–462` | `TestParity_TypeSberUnofficialModelsOperationDetailFieldL459` |
| `OperationDetail` | uoh_id: str; form: str; title: str; amount: Money \| None; state: str; state_name: str; state_description: str; statement_available: bool; fields: tuple[OperationDetailField, ...] | `sber_unofficial/models.py:466–475` | `TestParity_TypeSberUnofficialModelsOperationDetailL466` |
| `CardLimits` | purchase: Money \| None; available: Money \| None; available_total: Money \| None | `sber_unofficial/models.py:508–511` | `TestParity_TypeSberUnofficialModelsCardLimitsL508` |
| `CreditInfo` | limit: Money \| None; own_sum: Money \| None; debt: Money \| None; min_payment: Money \| None; min_payment_date: str | `sber_unofficial/models.py:515–520` | `TestParity_TypeSberUnofficialModelsCreditInfoL515` |
| `CardInfo` | id: str; name: str; last4: str; state: str; card_holder: str; pay_system_type: str; expire_date: str; limits: CardLimits; credit: CreditInfo \| None | `sber_unofficial/models.py:524–533` | `TestParity_TypeSberUnofficialModelsCardInfoL524` |
| `CategoryAmount` | id: str; name: str; external_id: str; national_amount: Money \| None; visible_amount: Money \| None; count_operations: int | `sber_unofficial/models.py:577–583` | `TestParity_TypeSberUnofficialModelsCategoryAmountL577` |
| `PfmPeriod` | from_: str; to: str; income_type: str; national_amount: Money \| None; visible_amount: Money \| None; categories: tuple[CategoryAmount, ...] | `sber_unofficial/models.py:587–593` | `TestParity_TypeSberUnofficialModelsPfmPeriodL587` |
| `PfmAmounts` | periods: tuple[PfmPeriod, ...] | `sber_unofficial/models.py:597–598` | `TestParity_TypeSberUnofficialModelsPfmAmountsL597` |
| `_PinRuntime` | base_url: str; process_id: str; pin_length: int; n_hex: str; g_hex: str; seamless_web: bool; redirect_post: bool | `sber_unofficial/pin.py:47–54` | `TestParity_TypeSberUnofficialPinPinRuntimeL47` |
| `PinSrp` | See base types, constructors and callable contracts | `sber_unofficial/pin.py:57–136` | `TestParity_TypeSberUnofficialPinPinSrpL57` |
| `AsyncPinAuth` | See base types, constructors and callable contracts | `sber_unofficial/pin.py:139–665` | `TestParity_TypeSberUnofficialPinAsyncPinAuthL139` |
| `AsyncPrimaryAuth` | See base types, constructors and callable contracts | `sber_unofficial/pin.py:668–1020` | `TestParity_TypeSberUnofficialPinAsyncPrimaryAuthL668` |
| `_Client` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:60–84` | `TestParity_TypeSberUnofficialResourcesClientL60` |
| `ProductsAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:87–97` | `TestParity_TypeSberUnofficialResourcesProductsAPIL87` |
| `AccountsAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:100–105` | `TestParity_TypeSberUnofficialResourcesAccountsAPIL100` |
| `OperationsAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:108–214` | `TestParity_TypeSberUnofficialResourcesOperationsAPIL108` |
| `CardsAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:217–256` | `TestParity_TypeSberUnofficialResourcesCardsAPIL217` |
| `AnalyticsAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:259–300` | `TestParity_TypeSberUnofficialResourcesAnalyticsAPIL259` |
| `TransfersAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:303–493` | `TestParity_TypeSberUnofficialResourcesTransfersAPIL303` |
| `SessionAPI` | See base types, constructors and callable contracts | `sber_unofficial/resources.py:496–511` | `TestParity_TypeSberUnofficialResourcesSessionAPIL496` |
| `CookieRecord` | name: str; value: str = field(repr=False); domain: str; path: str = '/'; secure: bool = True; http_only: bool = False; host_only: bool = True; expires: int \| None = None; same_site: str \| None = None | `sber_unofficial/session.py:40–83` | `TestParity_TypeSberUnofficialSessionCookieRecordL40` |
| `BrowserProfile` | headers: tuple[tuple[str, str], ...] = field(default_factory=tuple, repr=False) | `sber_unofficial/session.py:87–129` | `TestParity_TypeSberUnofficialSessionBrowserProfileL87` |
| `SberCredentials` | ufs_session: str = field(repr=False); ufs_token: str = field(repr=False) | `sber_unofficial/session.py:133–217` | `TestParity_TypeSberUnofficialSessionSberCredentialsL133` |
| `SessionBundle` | api_base: str; web_base: str; cookies: tuple[CookieRecord, ...] = field(repr=False); browser: BrowserProfile = field(default_factory=BrowserProfile, repr=False); deviceprint: str \| None = field(default=None, repr=False); antifraud_deviceprint: str \| None = field(default=None, repr=False); captured_at: str \| None = None; schema_version: int = SESSION_SCHEMA_VERSION | `sber_unofficial/session.py:221–604` | `TestParity_TypeSberUnofficialSessionSessionBundleL221` |
| `ResponseLike` | status_code: int; headers: Mapping[str, str]; text: str; content: bytes | `sber_unofficial/transport.py:24–30` | `TestParity_TypeSberUnofficialTransportResponseLikeL24` |
| `AsyncTransport` | See base types, constructors and callable contracts | `sber_unofficial/transport.py:33–62` | `TestParity_TypeSberUnofficialTransportAsyncTransportL33` |
| `_ResponseCookieJar` | See base types, constructors and callable contracts | `sber_unofficial/transport.py:69–79` | `TestParity_TypeSberUnofficialTransportResponseCookieJarL69` |
| `_CookieMetadataSession` | See base types, constructors and callable contracts | `sber_unofficial/transport.py:82–112` | `TestParity_TypeSberUnofficialTransportCookieMetadataSessionL82` |
| `CurlAsyncTransport` | See base types, constructors and callable contracts | `sber_unofficial/transport.py:115–232` | `TestParity_TypeSberUnofficialTransportCurlAsyncTransportL115` |
| `_DemoOperations` | See base types, constructors and callable contracts | `mcp/sber_unofficial_mcp/demo.py:91–105` | `TestParity_TypeMcpSberUnofficialMcpDemoDemoOperationsL91` |
| `DemoClient` | See base types, constructors and callable contracts | `mcp/sber_unofficial_mcp/demo.py:108–124` | `TestParity_TypeMcpSberUnofficialMcpDemoDemoClientL108` |
| `Config` | profile: str; home: Path | `mcp/sber_unofficial_mcp/server.py:387–481` | `TestParity_TypeMcpSberUnofficialMcpServerConfigL387` |
| `PreparedRecord` | token: str; prepared: Any; review: dict; destination_id: str; amount_text: str; audit_id: str; expires_at: datetime; spent: bool = False; outcome: str = 'prepared'; task: asyncio.Task \| None = None; result: dict \| None = None | `mcp/sber_unofficial_mcp/server.py:488–499` | `TestParity_TypeMcpSberUnofficialMcpServerPreparedRecordL488` |
| `SessionState` | session_id: str; profile: str; mode: str; state: str; auth: AsyncPrimaryAuth \| AsyncPinAuth \| None = None; client: AsyncSberClient \| None = None; expects: list[str] = field(default_factory=list); inbox_path: Path \| None = None; inbox_inode: tuple[int, int] \| None = None; inbox_fd: int \| None = None; challenge_code: str \| None = None; challenge_expires: datetime \| None = None; captcha_artifact: Path \| None = None; captcha_challenge: Any = None; consumed: bool = False; drafts: dict[str, Any] = field(default_factory=dict); prepared: dict[str, PreparedRecord] = field(default_factory=dict); unresolved_audit: str \| None = None; lock: asyncio.Lock = field(default_factory=asyncio.Lock); created_at: datetime = field(default_factory=_now); last_activity: datetime = field(default_factory=_now) | `mcp/sber_unofficial_mcp/server.py:503–524` | `TestParity_TypeMcpSberUnofficialMcpServerSessionStateL503` |
| `Registry` | See base types, constructors and callable contracts | `mcp/sber_unofficial_mcp/server.py:527–689` | `TestParity_TypeMcpSberUnofficialMcpServerRegistryL527` |
| `_NotFound` | See base types, constructors and callable contracts | `mcp/sber_unofficial_mcp/server.py:692–693` | `TestParity_TypeMcpSberUnofficialMcpServerNotFoundL692` |
| `_Ambiguous` | See base types, constructors and callable contracts | `mcp/sber_unofficial_mcp/server.py:696–700` | `TestParity_TypeMcpSberUnofficialMcpServerAmbiguousL696` |
| `_InboxReplaced` | See base types, constructors and callable contracts | `mcp/sber_unofficial_mcp/server.py:703–704` | `TestParity_TypeMcpSberUnofficialMcpServerInboxReplacedL703` |
| `ResponseLike` | status_code: int; headers: Mapping[str, str] | `sber_client.py:128–132` | `TestParity_TypeSberClientResponseLikeL128` |
| `SessionLike` | See base types, constructors and callable contracts | `sber_client.py:135–137` | `TestParity_TypeSberClientSessionLikeL135` |
| `OperationsPage` | operations: list[Operation]; next_offset: int \| None | `sber_client.py:140–142` | `TestParity_TypeSberClientOperationsPageL140` |
| `SberClient` | See base types, constructors and callable contracts | `sber_client.py:170–309` | `TestParity_TypeSberClientSberClientL170` |

## Consequential seams and reference gaps

Reference gaps below are **not** silently reproduced or counted as working behavior. They require explicit Go regression tests and documented safe intent. Every seam is `pending`.

### Native Go entire SDK+CLI+MCP, no Python subprocess

**Source:** `sber_unofficial/__init__.py:57–115`, `pyproject.toml:55–68`

- Every AST public callable, explicit export, inherited method, dataclass field and MCP tool must have a documented working native equivalent; helper/interface mappings explicit.
- No stub counted as parity; all criteria initially pending.

**Required Go tests:** `TestParityNativeRuntimeHasNoPythonAdapter`, `TestParityPublicSurfaceInventoryMatches`

### Read-only defaults without dropping mutation parity

**Source:** `sber_unofficial/client.py:88–94`, `sber_unofficial/resources.py:224–493`, `mcp/sber_unofficial_mcp/server.py:801–825`, `mcp/sber_unofficial_mcp/server.py:1509–1557`

- All reads and write contracts retained; default Go write policy deny; explicit mutation opt-in, human confirmation for financial confirm.
- MCP default9/full14/demo6 tools, source five write tools absent from default schema.
- Default-deny facade is not a narrower bank-side session permission.
- **Observed gap/boundary:** Python SDK directly attaches TransfersAPI; only MCP registration gates writes. Go adds required policy intentionally, not exact unsafe behavior parity.

**Required Go tests:** `TestParityDefaultPolicyDeniesMutations`, `TestParityOptInRetainsAllFiveWriteTools`, `TestParityDemoNeverRegistersAuthOrWrites`

### Validated HTML/cookies/identity/process atomicity

**Source:** `sber_unofficial/pin.py:374–445`, `tests/test_bootstrap_seed_boundary.py:19–86`, `tests/test_bootstrap_atomicity.py:16–81`

- Use owner cookie snapshot before failed probe, never challenge cookies from that failed curl request.
- Validate complete rendered document, exact URL, browser identity and full cookie routing before atomic adoption; reset process/CSRF/SRP/OTP state before cleanup await.
- Reject close/cancellation race and malformed state without resurrection.

**Required Go tests:** `TestParityBootstrapSeedBeforeProbe`, `TestParityBootstrapAtomicAdoption`, `TestParityBootstrapCloseNeverResurrects`

### SameSite/domain/path/deletion and concurrent response fidelity

**Source:** `sber_unofficial/transport.py:65–112`, `sber_unofficial/session.py:518–549`, `tests/test_transport_cookie_metadata.py:56–167`

- Preserve exact opaque cookie names/values/scoping, host-only/expiry/HttpOnly/SameSite and prefixes.
- Native cookie dump must not erase untouched SameSite; authoritative Set-Cookie with no SameSite clears old policy even same value.
- Deletion scoped by name/domain/path; unrelated concurrently added cookies survive. Reject unrepresentable metadata, no silent unsupported-policy normalization.

**Required Go tests:** `TestParityCookieSameSiteAuthorityAndDeletion`, `TestParityCookieConcurrentReconciliation`, `TestParityCookieRoutingPrefixMetadata`

### Frontend literal parser and exact SRP/RSA flavors

**Source:** `sber_unofficial/pin.py:57–136`, `sber_unofficial/pin.py:1023–1314`, `sber_unofficial/pin.py:1575–1681`

- No JS eval; unique literal assignment/properties, comments/braces/strings handled, strict JSON literals for security-critical strings/bools/int; URL/group safety.
- SHA512 SRP minimal-vs-pad integer encodings and zero cases verified against existing known answer.
- RSA OAEP SHA1/MGF1SHA1/empty label, SPKI/PKCS1 strict DER, encryption width and random seed shape.
- **Observed gap/boundary:** No genuine RSA-OAEP key/seed/ciphertext known-answer exists in the audited tests. Existing mock is explicitly not crypto proof.

**Required Go tests:** `TestParityStrictFrontendLiteralParser`, `TestParitySRPFrontendKnownAnswer`, `TestParityRSAOAEPForgeSHA1KnownAnswerPendingSource`, `FuzzParityFrontendConfig`, `FuzzParityRSAKeyDER`

### Primary/PIN/OTP/remembered device lifecycle

**Source:** `sber_unofficial/pin.py:229–368`, `sber_unofficial/pin.py:703–877`, `tests/test_primary_auth.py:382–643`

- Retain direct redirect, OTP, wrong code retry, resend limit flags vs terminal state, new PIN setup, PIN skip, captcha image/audio, required unsupported WebAuthn and process-reset semantics.
- Clear strings/sensitive tracebacks; preserve public key only before accepted PIN; keep exact token/CSRF/cookie rotation.

**Required Go tests:** `TestParityPrimaryAndPinStateMachines`, `TestParityAuthSecretsNeverLeak`, `TestParityPinKeySpentOnlyAfterAcceptance`

### Native transport trust/header/browser continuity

**Source:** `sber_unofficial/transport.py:118–153`, `sber_unofficial/bootstrap.py:104–192`, `tests/test_bootstrap_factories.py:23–162`

- Verified TLS+hostname, no proxy/environment trust or browser sandbox bypass; explicit CA survives constructors/handoff/renewal.
- No automatic redirects or POST retry. Native implementation must test browser→HTTP identity/cookie continuity rather than claim stock net/http is equivalent.
- **Observed gap/boundary:** This run used no bank/network; public continuity and owner auth remain unverified external evidence levels.

**Required Go tests:** `TestParityTransportTrustEnvironmentAndRetryDisabled`, `TestParityCAAcrossFactoriesHandoffAndRenewal`, `TestParityBrowserHeaderIdentityContinuity`

### Read-only renewal generations and close ownership

**Source:** `sber_unofficial/client.py:255–283`, `sber_unofficial/client.py:372–413`, `tests/test_profile_reauth_edge_cases.py:185–392`

- Exactly one PIN renewal for concurrent requests in one rejected generation; retry read once; later generation may renew; errors keep valid original profile where possible.
- Persist before transport swap, close abandoned replacements and retry retired cleanup; aclose releases provider, never resurrects transport.

**Required Go tests:** `TestParityConcurrentReadReauthEpoch`, `TestParityReauthPersistBeforeSwap`, `TestParityOwnedTransportCloseRace`

### Write one-shot no replay under failure/cancellation

**Source:** `sber_unofficial/client.py:285–370`, `sber_unofficial/resources.py:425–493`, `tests/test_mutations.py:361–612`

- No read-style renewal/replay for any write. One-shot draft/confirm guards and whole 3POST lock.
- Distinguish definite business rejection from unknown sent result; after send cancellation/save failure/invalid finish marks uncertainty; reconcile using reads only.

**Required Go tests:** `TestParityMutationNeverReauthenticatesOrReplays`, `TestParityWriteCancelAndSaveFailureUncertain`, `TestParityConfirmationOneShotAtomicSequence`

### Exact decimals and complete history, unknown not unpaid

**Source:** `sber_unofficial/models.py:144–163`, `sber_unofficial/models.py:281–344`, `sber_unofficial/resources.py:112–207`, `tests/test_operation_semantics.py:7–72`

- Never confuse operationAmount with resource billingAmount/balance_after; nullable optional values/unknown isFinancial remain distinct.
- N+1 sequential pages, stable bounds/dedup/date order; missingID or page guard unavailable = UNKNOWN, not zero or unpaid.
- Return count/truncated/next-offset consistently; a completed synthetic test does not prove authorized real history.

**Required Go tests:** `TestParityOperationAmountVersusBalanceAfter`, `TestParityHistoryCompletePaginationFailureUnknown`, `TestParityExactDecimalNoFloatMoney`, `FuzzParityMoneyAndHistoryParser`

### Portable private state and schema migrations

**Source:** `sber_unofficial/session.py:397–512`, `sber_unofficial/session.py:671–787`, `tests/test_profile_schema.py:14–146`

- Schema1/2/3→4 exact fields, duplicate JSON rejects, bounded owner/private/no-follow loads, exact type checks.
- Atomic0600 fsynced secret saves and failure cleanup; no symlink-following or unsupported-platform owner-check fallback.

**Required Go tests:** `TestParitySessionV1V2V3ToV4`, `TestParitySecretFileNoFollowOwnerSizeAndSchema`, `TestParitySecretSaveAtomicCleanup`

### Owner hidden input, lock and no-replace enrollment

**Source:** `../local_sber.py:22–121`, `sber_unofficial/__main__.py:143–191`, `mcp/sber_unofficial_mcp/server.py:1763–1771`

- Bootstrap succeeds before first secret prompt; input hidden only on terminal, fail closed if echo control unavailable.
- 0700 owner state; stable0600 regular single-link inode with nonblocking flock/dev+inode check; candidate atomic0600 writer then hard-link no-replace; refuse existing file/symlink.
- **Observed gap/boundary:** Audited fork tests cover SDK CLI, not the local safe launcher. No launcher golden was invented; dedicated Go regression tests are pending.
- **Observed gap/boundary:** SDK/MCP fallback getpass/login prompts permit echoing; required Go safety intentionally supersedes this.

**Required Go tests:** `TestParityOwnerTTYHiddenFailClosed`, `TestParityBootstrapBeforeFirstSecret`, `TestParityEnrollmentLockConcurrentNoReplace`, `TestParityEnrollmentRejectsSymlinkHardlinkAndOwnerMismatch`

### MCP14-tool schema, static secret-free routing

**Source:** `mcp/sber_unofficial_mcp/server.py:163–360`, `mcp/sber_unofficial_mcp/server.py:1509–1569`, `tests/test_mcp_server.py:43–92`, `tests/test_mcp_server.py:303–334`

- All14 input signatures exact; no login/password/PIN/OTP/cookies/deviceprint fields, transient secrets via owner inbox only.
- 12 closed next_action values, static safe messages, structured bounded redacted fields; cancellation propagates, never raw exception string or traceback locals.
- **Observed gap/boundary:** Declared page_limit/internal session_busy/challenge_timeout messages are not all produced by live source code. Go must make explicit mapping decisions.

**Required Go tests:** `TestParityMCPToolDefinitionsAndNoSecretSchema`, `TestParityMCPErrorEnvelopeRoutingAndRedaction`, `TestParityMCPStdoutOnlyJSONRPC`

### MCP session locking and actual expiry lifecycle

**Source:** `mcp/sber_unofficial_mcp/server.py:503–525`, `mcp/sber_unofficial_mcp/server.py:673–689`, `mcp/sber_unofficial_mcp/server.py:1517–1558`, `mcp/sber_unofficial_mcp/server.py:1987–2010`

- Session resolution respects explicit/default/sole/ambiguous; max4 and profile isolation.
- Actually serialize session auth/transfer state, schedule and close TTL reaper; expired15min pending/30min idle auth cannot remain live.
- Enforce returned draft expiration and token300sec expiration; cleanup tasks/inbox/transports under cancellation.
- **Observed gap/boundary:** Source SessionState.lock is never acquired in _guard or tools; guard docstring promises serialization but code does not.
- **Observed gap/boundary:** reap_forever defined but not scheduled from main/build. Draft600sec expiry returned but not stored or checked.

**Required Go tests:** `TestParityMCPSessionStateSerialized`, `TestParityMCPReaperScheduledAndShutdownSafe`, `TestParityMCPDraftTTLActuallyEnforced`

### Single-use inbox and durable private audit

**Source:** `mcp/sber_unofficial_mcp/server.py:580–650`, `tests/test_mcp_server.py:99–128`

- Owner/private bounded inode-pinned secret input; detect replacement even inode reuse; consume/unlink before auth call.
- Existing audit file checked regular/current owner0600/no-follow/single link; durable complete JSONL writes; audit never carries secret values.
- **Observed gap/boundary:** Source audit opens0600/NOFOLLOW but does not validate existing mode/owner/link count or fsync/full-write result; must be explicit Go safety acceptance.

**Required Go tests:** `TestParityMCPInboxReplacementSingleUse`, `TestParityMCPAuditExistingFileOwnerModeAndDurability`

### Human review, acknowledgement, task shield and uncertainty latch

**Source:** `mcp/sber_unofficial_mcp/server.py:1347–1478`, `tests/test_mcp_server.py:192–244`

- Review shown verbatim, explicit human yes, session-bound expiring token, exact constant-time amount/destination acknowledgement; burn before send.
- One task; repeats never double-send; source uncertainty latch blocks writes until owner reconciliation; spent token never revives.
- Finalize shielded task result after caller cancellation; always terminal unknown once sent result uncertain, never generic retryable transport envelope for financial confirm.
- **Observed gap/boundary:** Source catches BaseException then latches uncertainty but may return generic mapped error and lacks robust cancelled waiter/shielded-task finalization.

**Required Go tests:** `TestParityMCPApprovalAndAcknowledgementGate`, `TestParityMCPTokenBurnAndNoDoubleSend`, `TestParityMCPShieldedConfirmCancellationFinalizes`, `TestParityMCPUncertainResultNeverRetryable`

### Explicit audio-captcha and demo completeness gaps

**Source:** `mcp/sber_unofficial_mcp/server.py:1054–1069`, `mcp/sber_unofficial_mcp/server.py:1134–1149`, `mcp/sber_unofficial_mcp/demo.py:94–124`, `mcp/sber_unofficial_mcp/server.py:1185–1208`

- Audio challenge continuation submits correct audioCaptchaCode and remembers selected mode.
- Demo remains network-free and every registered demo tool incl session_info(check_live) has coherent synthetic response, marked as demo; enforce/document demo filter semantics.
- **Observed gap/boundary:** Source audio path refetches audio but _advance_captcha always submits captcha_code.
- **Observed gap/boundary:** DemoClient bundle SimpleNamespace lacks redacted() and warm_up(); session_info may fail although registered. Demo operations ignore resource/date filters intentionally.

**Required Go tests:** `TestParityMCPAudioCaptchaContinuation`, `TestParityMCPDemoEveryRegisteredToolOffline`

### Deprecated shims mapped explicitly, not silently deleted

**Source:** `sber_client.py:170–309`, `outputs/sber_parser.py:12–50`, `tests/test_compat_shims.py:11–29`

- Go equivalents for legacy read/offline-parser symbols and CLI paths explicitly documented; no Python runtime import.
- Legacy sync warmup starts at construction timestamp and automatic60sec warmups, invalid JSON=>expired, success=false=>ApiError, duplicate-last/raw date sort differ from async; retain or explicitly adapt with tests.
- **Observed gap/boundary:** Canonical async semantics govern new native API; unsafe legacy retry=2 must not bleed into mutation-capable transport.

**Required Go tests:** `TestParityLegacyExplicitNativeMapping`, `TestParityLegacyVsAsyncIntentionalDifferences`

### MIT attribution and evidence boundaries

**Source:** `LICENSE:1–21`, `sber_unofficial/__init__.py:55–55`

- Preserve MIT copyright/license notice in derived source/fixture/document copies.
- Inventory and offline fixtures are not Go implementation, public bank continuity, successful owner auth or complete authorized history.
- **Observed gap/boundary:** All Go verification statuses pending; no bank/network used and no repo API/commit/publication performed.

**Required Go tests:** `TestParityMITAttributionRetained`, `TestParityNoFalseLiveVerificationClaims`

## Synthetic fixtures and provenance

Only audited test literals/factories/parameter tables were copied or deterministically materialized. Model outputs are derived from the actual stdlib-only source on those test inputs; no provider response or owner device/cookie/profile was invented. Dates/IDs/cookie strings/PANs in these files are explicitly existing synthetic test values.

**RSA exception:** audited tests have a deterministic 256-byte mock ciphertext and invalid-key checks, but no genuine OAEP known-answer key/seed/ciphertext. `rsa.json` labels the mock as non-cryptographic; the real Go OAEP known-answer criterion stays pending rather than fabricating a fixture.

| Fixture file | Records | Provenance / purpose |
|---|---:|---|
| `testdata/compat/srp.json` | 1 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/rsa.json` | 2 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/frontend-config.json` | 9 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/resources.json` | 4 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/history.json` | 5 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/session.json` | 7 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/cookie-metadata.json` | 6 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/mutations.json` | 1 | Each record carries exact audited test file, line range, SHA-256, source test and required Go identifier |
| `testdata/compat/parameter-tables.json` | 30 parameterized test definitions | Literal axes copied; nonliteral expressions retained unevaluated |
| `testdata/compat/python-test-contracts.json` | 199 definitions | Complete assertions/expected raises/setup and fake helper sources; all pending |
| `testdata/compat/fixture-provenance.json` | index | File digests, family counts, checks and limitations |

## Complete source-test acceptance index

The 16 brand/frontend-snapshot maintenance tests are separately tagged, not represented as SDK runtime behavior or silently discarded. Reproduce or explicitly adapt maintenance intent without a Python runtime dependency.

| Source test | Parameter cases | Category | Source | Required Go test |
|---|---:|---|---|---|
| `test_credentials_build_exact_auth_cookie_set_and_roundtrip` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:101–119` | `TestParity_PythonTestTestsTestAsyncSdkTestCredentialsBuildExactAuthCookieSetAndRoundtripL101` |
| `test_credentials_reject_incomplete_or_unsafe_values` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:122–131` | `TestParity_PythonTestTestsTestAsyncSdkTestCredentialsRejectIncompleteOrUnsafeValuesL122` |
| `test_client_can_be_created_from_credentials_without_cookie_file` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:134–152` | `TestParity_PythonTestTestsTestAsyncSdkTestClientCanBeCreatedFromCredentialsWithoutCookieFileL134` |
| `test_credentials_client_autosaves_rotation_when_session_path_is_set` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:155–185` | `TestParity_PythonTestTestsTestAsyncSdkTestCredentialsClientAutosavesRotationWhenSessionPathIsSetL155` |
| `test_session_bundle_roundtrip_is_private_and_redacted` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:188–199` | `TestParity_PythonTestTestsTestAsyncSdkTestSessionBundleRoundtripIsPrivateAndRedactedL188` |
| `test_session_file_autosaves_rotated_minimal_credentials_only_after_success` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:202–263` | `TestParity_PythonTestTestsTestAsyncSdkTestSessionFileAutosavesRotatedMinimalCredentialsOnlyAfterSuccessL202` |
| `test_insecure_session_bundle_is_rejected` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:266–271` | `TestParity_PythonTestTestsTestAsyncSdkTestInsecureSessionBundleIsRejectedL266` |
| `test_session_bundle_rejects_symlink_and_duplicate_json_keys` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:274–286` | `TestParity_PythonTestTestsTestAsyncSdkTestSessionBundleRejectsSymlinkAndDuplicateJsonKeysL274` |
| `test_session_bundle_schema_requires_host_only_and_exact_header_fields` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:289–304` | `TestParity_PythonTestTestsTestAsyncSdkTestSessionBundleSchemaRequiresHostOnlyAndExactHeaderFieldsL289` |
| `test_failed_pre_replace_validation_leaves_no_secret_file` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:307–316` | `TestParity_PythonTestTestsTestAsyncSdkTestFailedPreReplaceValidationLeavesNoSecretFileL307` |
| `test_failed_post_replace_validation_removes_secret_file` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:319–334` | `TestParity_PythonTestTestsTestAsyncSdkTestFailedPostReplaceValidationRemovesSecretFileL319` |
| `test_nofollow_fallback_rejects_symlink` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:337–344` | `TestParity_PythonTestTestsTestAsyncSdkTestNofollowFallbackRejectsSymlinkL337` |
| `test_private_load_fails_closed_without_owner_check` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:347–352` | `TestParity_PythonTestTestsTestAsyncSdkTestPrivateLoadFailsClosedWithoutOwnerCheckL347` |
| `test_cookie_prefix_and_identity_validation` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:355–364` | `TestParity_PythonTestTestsTestAsyncSdkTestCookiePrefixAndIdentityValidationL355` |
| `test_curl_rotated_cookie_preserves_http_only_boolean` | 2 | synthetic_runtime_test | `tests/test_async_sdk.py:368–373` | `TestParity_PythonTestTestsTestAsyncSdkTestCurlRotatedCookiePreservesHttpOnlyBooleanL368` |
| `test_full_curl_roundtrip_keeps_existing_http_only_metadata` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:376–383` | `TestParity_PythonTestTestsTestAsyncSdkTestFullCurlRoundtripKeepsExistingHttpOnlyMetadataL376` |
| `test_cookie_rotation_coalesces_equivalent_dotted_domains` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:386–400` | `TestParity_PythonTestTestsTestAsyncSdkTestCookieRotationCoalescesEquivalentDottedDomainsL386` |
| `test_concurrent_business_calls_do_not_depend_on_legacy_warmup` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:403–415` | `TestParity_PythonTestTestsTestAsyncSdkTestConcurrentBusinessCallsDoNotDependOnLegacyWarmupL403` |
| `test_concurrent_products_share_frontend_api_discovery` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:418–469` | `TestParity_PythonTestTestsTestAsyncSdkTestConcurrentProductsShareFrontendApiDiscoveryL418` |
| `test_successful_warmup_persists_discovery_and_rotated_credentials` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:472–509` | `TestParity_PythonTestTestsTestAsyncSdkTestSuccessfulWarmupPersistsDiscoveryAndRotatedCredentialsL472` |
| `test_async_operations_n_plus_one_pagination_is_sequential` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:512–532` | `TestParity_PythonTestTestsTestAsyncSdkTestAsyncOperationsNPlusOnePaginationIsSequentialL512` |
| `test_redirect_is_authentication_expired` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:535–549` | `TestParity_PythonTestTestsTestAsyncSdkTestRedirectIsAuthenticationExpiredL535` |
| `test_response_errors_are_not_misclassified_as_expired_auth` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:552–563` | `TestParity_PythonTestTestsTestAsyncSdkTestResponseErrorsAreNotMisclassifiedAsExpiredAuthL552` |
| `test_operation_without_uoh_id_fails_closed` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:566–575` | `TestParity_PythonTestTestsTestAsyncSdkTestOperationWithoutUohIdFailsClosedL566` |
| `test_failed_close_can_be_retried` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:578–599` | `TestParity_PythonTestTestsTestAsyncSdkTestFailedCloseCanBeRetriedL578` |
| `test_curl_transport_pins_ca_and_disables_environment_proxy` | 1 | synthetic_runtime_test | `tests/test_async_sdk.py:602–616` | `TestParity_PythonTestTestsTestAsyncSdkTestCurlTransportPinsCaAndDisablesEnvironmentProxyL602` |
| `test_bootstrap_resets_all_process_state_before_old_transport_close` | 2 | synthetic_runtime_test | `tests/test_bootstrap_atomicity.py:16–55` | `TestParity_PythonTestTestsTestBootstrapAtomicityTestBootstrapResetsAllProcessStateBeforeOldTransportCloseL16` |
| `test_malformed_callback_state_is_rejected_before_any_adoption` | 3 | synthetic_runtime_test | `tests/test_bootstrap_atomicity.py:59–81` | `TestParity_PythonTestTestsTestBootstrapAtomicityTestMalformedCallbackStateIsRejectedBeforeAnyAdoptionL59` |
| `test_browser_samesite_survives_full_roundtrip_and_migrates_v3` | 3 | synthetic_runtime_test | `tests/test_bootstrap_cookie_metadata.py:17–33` | `TestParity_PythonTestTestsTestBootstrapCookieMetadataTestBrowserSamesiteSurvivesFullRoundtripAndMigratesV3L17` |
| `test_cookie_rejects_unsupported_samesite` | 4 | synthetic_runtime_test | `tests/test_bootstrap_cookie_metadata.py:37–40` | `TestParity_PythonTestTestsTestBootstrapCookieMetadataTestCookieRejectsUnsupportedSamesiteL37` |
| `test_browser_cookie_import_preserves_samesite_instead_of_dropping_it` | 1 | synthetic_runtime_test | `tests/test_bootstrap_cookie_metadata.py:43–51` | `TestParity_PythonTestTestsTestBootstrapCookieMetadataTestBrowserCookieImportPreservesSamesiteInsteadOfDroppingItL43` |
| `test_response_samesite_case_variants_are_exported_canonically` | 28 | synthetic_runtime_test | `tests/test_bootstrap_cookie_metadata.py:66–73` | `TestParity_PythonTestTestsTestBootstrapCookieMetadataTestResponseSamesiteCaseVariantsAreExportedCanonicallyL66` |
| `test_response_invalid_samesite_is_not_silently_normalized` | 16 | synthetic_runtime_test | `tests/test_bootstrap_cookie_metadata.py:78–82` | `TestParity_PythonTestTestsTestBootstrapCookieMetadataTestResponseInvalidSamesiteIsNotSilentlyNormalizedL78` |
| `test_authoritative_set_cookie_without_samesite_removes_previous_policy` | 2 | synthetic_runtime_test | `tests/test_bootstrap_cookie_metadata.py:86–94` | `TestParity_PythonTestTestsTestBootstrapCookieMetadataTestAuthoritativeSetCookieWithoutSamesiteRemovesPreviousPolicyL86` |
| `test_explicit_ca_never_disables_tls_proxy_redirect_or_post_retry` | 1 | synthetic_runtime_test | `tests/test_bootstrap_factories.py:23–53` | `TestParity_PythonTestTestsTestBootstrapFactoriesTestExplicitCaNeverDisablesTlsProxyRedirectOrPostRetryL23` |
| `test_ca_survives_initial_auth_and_browser_handoff` | 2 | synthetic_runtime_test | `tests/test_bootstrap_factories.py:57–87` | `TestParity_PythonTestTestsTestBootstrapFactoriesTestCaSurvivesInitialAuthAndBrowserHandoffL57` |
| `test_pin_profile_retains_provider_timeout_and_ca_through_initial_and_later_reauth` | 2 | synthetic_runtime_test | `tests/test_bootstrap_factories.py:91–137` | `TestParity_PythonTestTestsTestBootstrapFactoriesTestPinProfileRetainsProviderTimeoutAndCaThroughInitialAndLaterReauthL91` |
| `test_client_construction_factories_keep_explicit_ca` | 3 | synthetic_runtime_test | `tests/test_bootstrap_factories.py:141–162` | `TestParity_PythonTestTestsTestBootstrapFactoriesTestClientConstructionFactoriesKeepExplicitCaL141` |
| `test_shutdown_rejects_pending_bootstrap_without_resurrecting_transport` | 12 | synthetic_runtime_test | `tests/test_bootstrap_lifetime.py:18–91` | `TestParity_PythonTestTestsTestBootstrapLifetimeTestShutdownRejectsPendingBootstrapWithoutResurrectingTransportL18` |
| `test_concurrent_shutdown_during_retired_transport_cleanup_is_safe` | 2 | synthetic_runtime_test | `tests/test_bootstrap_lifetime.py:95–142` | `TestParity_PythonTestTestsTestBootstrapLifetimeTestConcurrentShutdownDuringRetiredTransportCleanupIsSafeL95` |
| `test_completed_auth_close_does_not_close_custom_transport_twice` | 1 | synthetic_runtime_test | `tests/test_bootstrap_lifetime.py:145–162` | `TestParity_PythonTestTestsTestBootstrapLifetimeTestCompletedAuthCloseDoesNotCloseCustomTransportTwiceL145` |
| `test_browser_extra_is_optional_and_public_exports_are_lazy` | 1 | synthetic_runtime_test | `tests/test_bootstrap_optional_dependency.py:8–23` | `TestParity_PythonTestTestsTestBootstrapOptionalDependencyTestBrowserExtraIsOptionalAndPublicExportsAreLazyL8` |
| `test_browser_seed_is_current_cookie_snapshot_before_failed_curl_probe` | 4 | synthetic_runtime_test | `tests/test_bootstrap_seed_boundary.py:19–86` | `TestParity_PythonTestTestsTestBootstrapSeedBoundaryTestBrowserSeedIsCurrentCookieSnapshotBeforeFailedCurlProbeL19` |
| `test_committed_output_is_current` | 1 | publication_maintenance_test | `tests/test_brand.py:38–47` | `TestParity_PythonTestTestsTestBrandTestCommittedOutputIsCurrentL38` |
| `test_no_orphaned_assets` | 1 | publication_maintenance_test | `tests/test_brand.py:50–54` | `TestParity_PythonTestTestsTestBrandTestNoOrphanedAssetsL50` |
| `test_build_is_deterministic` | 1 | publication_maintenance_test | `tests/test_brand.py:57–61` | `TestParity_PythonTestTestsTestBrandTestBuildIsDeterministicL57` |
| `test_every_tool_has_a_group` | 1 | publication_maintenance_test | `tests/test_brand.py:64–73` | `TestParity_PythonTestTestsTestBrandTestEveryToolHasAGroupL64` |
| `test_unknown_tool_prefix_is_rejected` | 1 | publication_maintenance_test | `tests/test_brand.py:76–78` | `TestParity_PythonTestTestsTestBrandTestUnknownToolPrefixIsRejectedL76` |
| `test_every_group_is_labelled_in_every_language` | 1 | publication_maintenance_test | `tests/test_brand.py:81–93` | `TestParity_PythonTestTestsTestBrandTestEveryGroupIsLabelledInEveryLanguageL81` |
| `test_a_growing_group_still_fits_its_panel` | 1 | publication_maintenance_test | `tests/test_brand.py:96–121` | `TestParity_PythonTestTestsTestBrandTestAGrowingGroupStillFitsItsPanelL96` |
| `test_charts_degrade_instead_of_crashing` | 1 | publication_maintenance_test | `tests/test_brand.py:124–144` | `TestParity_PythonTestTestsTestBrandTestChartsDegradeInsteadOfCrashingL124` |
| `test_graphics_carry_no_untranslated_english` | 1 | publication_maintenance_test | `tests/test_brand.py:147–157` | `TestParity_PythonTestTestsTestBrandTestGraphicsCarryNoUntranslatedEnglishL147` |
| `test_facts_match_the_source` | 1 | publication_maintenance_test | `tests/test_brand.py:160–167` | `TestParity_PythonTestTestsTestBrandTestFactsMatchTheSourceL160` |
| `test_readme_states_the_real_tool_count` | 1 | publication_maintenance_test | `tests/test_brand.py:170–174` | `TestParity_PythonTestTestsTestBrandTestReadmeStatesTheRealToolCountL170` |
| `test_svg_is_well_formed` | 1 | publication_maintenance_test | `tests/test_brand.py:177–185` | `TestParity_PythonTestTestsTestBrandTestSvgIsWellFormedL177` |
| `test_languages_are_structurally_identical` | 1 | publication_maintenance_test | `tests/test_brand.py:189–214` | `TestParity_PythonTestTestsTestBrandTestLanguagesAreStructurallyIdenticalL189` |
| `test_templates_use_the_same_placeholders` | 1 | publication_maintenance_test | `tests/test_brand.py:217–224` | `TestParity_PythonTestTestsTestBrandTestTemplatesUseTheSamePlaceholdersL217` |
| `test_no_real_looking_secrets_in_brand_content` | 1 | publication_maintenance_test | `tests/test_brand.py:227–232` | `TestParity_PythonTestTestsTestBrandTestNoRealLookingSecretsInBrandContentL227` |
| `test_valid_config_with_tspd_boilerplate_is_not_misclassified_as_a_check` | 2 | synthetic_runtime_test | `tests/test_browser_bootstrap.py:63–76` | `TestParity_PythonTestTestsTestBrowserBootstrapTestValidConfigWithTspdBoilerplateIsNotMisclassifiedAsACheckL63` |
| `test_http_200_browser_check_is_actionable_and_never_submits_credentials` | 2 | synthetic_runtime_test | `tests/test_browser_bootstrap.py:80–92` | `TestParity_PythonTestTestsTestBrowserBootstrapTestHttp200BrowserCheckIsActionableAndNeverSubmitsCredentialsL80` |
| `test_opt_in_bootstrap_atomically_adopts_valid_html_cookies_and_identity` | 2 | synthetic_runtime_test | `tests/test_browser_bootstrap.py:96–148` | `TestParity_PythonTestTestsTestBrowserBootstrapTestOptInBootstrapAtomicallyAdoptsValidHtmlCookiesAndIdentityL96` |
| `test_failed_optional_bootstrap_keeps_original_state_and_redacts_failures` | 16 | synthetic_runtime_test | `tests/test_browser_bootstrap_failures.py:19–74` | `TestParity_PythonTestTestsTestBrowserBootstrapFailuresTestFailedOptionalBootstrapKeepsOriginalStateAndRedactsFailuresL19` |
| `test_bootstrap_cancellation_propagates_without_credentials_or_adoption` | 2 | synthetic_runtime_test | `tests/test_browser_bootstrap_failures.py:78–105` | `TestParity_PythonTestTestsTestBrowserBootstrapFailuresTestBootstrapCancellationPropagatesWithoutCredentialsOrAdoptionL78` |
| `test_enroll_uses_private_inputs_and_saves_mode_0600` | 1 | synthetic_runtime_test | `tests/test_cli.py:88–135` | `TestParity_PythonTestTestsTestCliTestEnrollUsesPrivateInputsAndSavesMode0600L88` |
| `test_enroll_rejects_mismatched_pin_before_create` | 1 | synthetic_runtime_test | `tests/test_cli.py:138–160` | `TestParity_PythonTestTestsTestCliTestEnrollRejectsMismatchedPinBeforeCreateL138` |
| `test_enroll_accepts_direct_session_without_pin_creation` | 1 | synthetic_runtime_test | `tests/test_cli.py:163–192` | `TestParity_PythonTestTestsTestCliTestEnrollAcceptsDirectSessionWithoutPinCreationL163` |
| `test_enroll_continues_primary_login_after_captcha` | 1 | synthetic_runtime_test | `tests/test_cli.py:195–259` | `TestParity_PythonTestTestsTestCliTestEnrollContinuesPrimaryLoginAfterCaptchaL195` |
| `test_network_commands_read_pin_from_keychain_without_exposing_it` | 2 | synthetic_runtime_test | `tests/test_cli.py:296–347` | `TestParity_PythonTestTestsTestCliTestNetworkCommandsReadPinFromKeychainWithoutExposingItL296` |
| `test_network_command_requires_keychain_options_as_a_pair` | 2 | synthetic_runtime_test | `tests/test_cli.py:354–364` | `TestParity_PythonTestTestsTestCliTestNetworkCommandRequiresKeychainOptionsAsAPairL354` |
| `test_network_command_keeps_session_file_fallback` | 1 | synthetic_runtime_test | `tests/test_cli.py:367–388` | `TestParity_PythonTestTestsTestCliTestNetworkCommandKeepsSessionFileFallbackL367` |
| `test_sber_client_shim_reexports_canonical_objects` | 1 | synthetic_runtime_test | `tests/test_compat_shims.py:11–20` | `TestParity_PythonTestTestsTestCompatShimsTestSberClientShimReexportsCanonicalObjectsL11` |
| `test_outputs_shim_reexports_canonical_objects` | 1 | synthetic_runtime_test | `tests/test_compat_shims.py:23–29` | `TestParity_PythonTestTestsTestCompatShimsTestOutputsShimReexportsCanonicalObjectsL23` |
| `test_deviceprint_shape_and_uniqueness` | 1 | synthetic_runtime_test | `tests/test_deviceprint.py:8–14` | `TestParity_PythonTestTestsTestDeviceprintTestDeviceprintShapeAndUniquenessL8` |
| `test_antifraud_is_percent_encoded_form` | 1 | synthetic_runtime_test | `tests/test_deviceprint.py:17–22` | `TestParity_PythonTestTestsTestDeviceprintTestAntifraudIsPercentEncodedFormL17` |
| `test_parser_keeps_both_account_kinds_and_links_only_unique_ctaccount` | 1 | synthetic_runtime_test | `tests/test_entities.py:46–70` | `TestParity_PythonTestTestsTestEntitiesTestParserKeepsBothAccountKindsAndLinksOnlyUniqueCtaccountL46` |
| `test_card_resolves_ctaccount_even_when_a_plain_account_shares_its_id` | 1 | synthetic_runtime_test | `tests/test_entities.py:73–83` | `TestParity_PythonTestTestsTestEntitiesTestCardResolvesCtaccountEvenWhenAPlainAccountSharesItsIdL73` |
| `test_bound_products_have_relations_methods_and_safe_serialization` | 1 | synthetic_runtime_test | `tests/test_entities.py:86–195` | `TestParity_PythonTestTestsTestEntitiesTestBoundProductsHaveRelationsMethodsAndSafeSerializationL86` |
| `test_client_and_resource_shortcuts_return_bound_snapshot` | 1 | synthetic_runtime_test | `tests/test_entities.py:198–223` | `TestParity_PythonTestTestsTestEntitiesTestClientAndResourceShortcutsReturnBoundSnapshotL198` |
| `test_firefox_provider_navigates_normally_with_verified_tls_and_exact_seed` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap.py:155–194` | `TestParity_PythonTestTestsTestFirefoxBootstrapTestFirefoxProviderNavigatesNormallyWithVerifiedTlsAndExactSeedL155` |
| `test_firefox_waits_for_normal_javascript_document_before_snapshot` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:25–36` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxWaitsForNormalJavascriptDocumentBeforeSnapshotL25` |
| `test_firefox_owner_required_failures_are_redacted_and_close_browser` | 4 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:40–73` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxOwnerRequiredFailuresAreRedactedAndCloseBrowserL40` |
| `test_firefox_rejects_unrepresentable_cookie_metadata` | 5 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:80–87` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxRejectsUnrepresentableCookieMetadataL80` |
| `test_firefox_rejects_unsafe_setup_before_launch` | 5 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:91–105` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxRejectsUnsafeSetupBeforeLaunchL91` |
| `test_firefox_safely_reports_missing_optional_dependency` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:108–116` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxSafelyReportsMissingOptionalDependencyL108` |
| `test_firefox_aborts_unapproved_requests_without_continuing` | 5 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:126–136` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxAbortsUnapprovedRequestsWithoutContinuingL126` |
| `test_firefox_seed_keeps_samesite_and_discards_expired_cookies` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:139–148` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxSeedKeepsSamesiteAndDiscardsExpiredCookiesL139` |
| `test_firefox_detects_visible_captcha_after_hidden_template` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:151–175` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxDetectsVisibleCaptchaAfterHiddenTemplateL151` |
| `test_firefox_rejects_executed_config_that_has_no_strict_literal_html` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:178–189` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxRejectsExecutedConfigThatHasNoStrictLiteralHtmlL178` |
| `test_firefox_waits_through_transient_document_navigation` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:192–205` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxWaitsThroughTransientDocumentNavigationL192` |
| `test_firefox_allows_ordinary_first_party_tspd_iframe_get` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:208–220` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxAllowsOrdinaryFirstPartyTspdIframeGetL208` |
| `test_firefox_subframe_cannot_replace_main_document_response_status` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:223–240` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxSubframeCannotReplaceMainDocumentResponseStatusL223` |
| `test_firefox_blocks_another_main_frame_bootstrap_process` | 1 | synthetic_runtime_test | `tests/test_firefox_bootstrap_failures.py:243–253` | `TestParity_PythonTestTestsTestFirefoxBootstrapFailuresTestFirefoxBlocksAnotherMainFrameBootstrapProcessL243` |
| `test_snapshot_keeps_static_get_and_drops_dynamic_secrets` | 1 | publication_maintenance_test | `tests/test_frontend_snapshot.py:26–48` | `TestParity_PythonTestTestsTestFrontendSnapshotTestSnapshotKeepsStaticGetAndDropsDynamicSecretsL26` |
| `test_map_error_never_leaks_and_always_routes` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:43–66` | `TestParity_PythonTestTestsTestMcpServerTestMapErrorNeverLeaksAndAlwaysRoutesL43` |
| `test_parse_error_maps_to_static_message` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:69–72` | `TestParity_PythonTestTestsTestMcpServerTestParseErrorMapsToStaticMessageL69` |
| `test_guard_never_raises_and_hides_exception_text` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:78–85` | `TestParity_PythonTestTestsTestMcpServerTestGuardNeverRaisesAndHidesExceptionTextL78` |
| `test_guard_reraises_cancellation` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:88–93` | `TestParity_PythonTestTestsTestMcpServerTestGuardReraisesCancellationL88` |
| `test_inbox_is_empty_then_single_use_then_consumed` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:99–112` | `TestParity_PythonTestTestsTestMcpServerTestInboxIsEmptyThenSingleUseThenConsumedL99` |
| `test_inbox_rejects_inode_replacement` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:115–125` | `TestParity_PythonTestTestsTestMcpServerTestInboxRejectsInodeReplacementL115` |
| `test_config_reports_insecure_permissions` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:131–148` | `TestParity_PythonTestTestsTestMcpServerTestConfigReportsInsecurePermissionsL131` |
| `test_transfer_confirm_burns_token_and_never_double_sends` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:192–207` | `TestParity_PythonTestTestsTestMcpServerTestTransferConfirmBurnsTokenAndNeverDoubleSendsL192` |
| `test_transfer_confirm_rejects_acknowledgement_mismatch_without_network` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:210–219` | `TestParity_PythonTestTestsTestMcpServerTestTransferConfirmRejectsAcknowledgementMismatchWithoutNetworkL210` |
| `test_uncertain_transfer_latches_session_and_blocks_further_transfers` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:222–245` | `TestParity_PythonTestTestsTestMcpServerTestUncertainTransferLatchesSessionAndBlocksFurtherTransfersL222` |
| `test_data_tools_resolve_active_session_without_session_id` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:248–254` | `TestParity_PythonTestTestsTestMcpServerTestDataToolsResolveActiveSessionWithoutSessionIdL248` |
| `test_resolve_is_ambiguous_only_without_a_default_profile` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:257–270` | `TestParity_PythonTestTestsTestMcpServerTestResolveIsAmbiguousOnlyWithoutADefaultProfileL257` |
| `test_demo_serves_a_synthetic_portfolio_with_no_login_and_no_network` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:276–291` | `TestParity_PythonTestTestsTestMcpServerTestDemoServesASyntheticPortfolioWithNoLoginAndNoNetworkL276` |
| `test_no_secret_named_field_in_any_tool_schema` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:303–312` | `TestParity_PythonTestTestsTestMcpServerTestNoSecretNamedFieldInAnyToolSchemaL303` |
| `test_writes_are_absent_from_the_schema_until_opted_in` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:316–324` | `TestParity_PythonTestTestsTestMcpServerTestWritesAreAbsentFromTheSchemaUntilOptedInL316` |
| `test_demo_exposes_reads_only_and_never_writes_even_if_asked` | 1 | synthetic_runtime_test | `tests/test_mcp_server.py:328–334` | `TestParity_PythonTestTestsTestMcpServerTestDemoExposesReadsOnlyAndNeverWritesEvenIfAskedL328` |
| `test_card_rename_matches_captured_contract_and_requires_deviceprint` | 1 | synthetic_runtime_test | `tests/test_mutations.py:134–159` | `TestParity_PythonTestTestsTestMutationsTestCardRenameMatchesCapturedContractAndRequiresDeviceprintL134` |
| `test_card_rename_reproduces_frontend_name_validation_before_network` | 1 | synthetic_runtime_test | `tests/test_mutations.py:162–175` | `TestParity_PythonTestTestsTestMutationsTestCardRenameReproducesFrontendNameValidationBeforeNetworkL162` |
| `test_card_rename_preserves_definite_business_rejection_fields` | 1 | synthetic_runtime_test | `tests/test_mutations.py:178–203` | `TestParity_PythonTestTestsTestMutationsTestCardRenamePreservesDefiniteBusinessRejectionFieldsL178` |
| `test_transfer_prepare_and_confirm_match_captured_five_request_workflow` | 1 | synthetic_runtime_test | `tests/test_mutations.py:206–282` | `TestParity_PythonTestTestsTestMutationsTestTransferPrepareAndConfirmMatchCapturedFiveRequestWorkflowL206` |
| `test_transfer_prepare_accepts_card_reference_returned_by_start` | 1 | synthetic_runtime_test | `tests/test_mutations.py:285–309` | `TestParity_PythonTestTestsTestMutationsTestTransferPrepareAcceptsCardReferenceReturnedByStartL285` |
| `test_transfer_validation_uses_only_start_references_before_network` | 1 | synthetic_runtime_test | `tests/test_mutations.py:312–358` | `TestParity_PythonTestTestsTestMutationsTestTransferValidationUsesOnlyStartReferencesBeforeNetworkL312` |
| `test_mutations_never_reauthenticate_or_replay_and_confirm_failure_is_uncertain` | 1 | synthetic_runtime_test | `tests/test_mutations.py:361–396` | `TestParity_PythonTestTestsTestMutationsTestMutationsNeverReauthenticateOrReplayAndConfirmFailureIsUncertainL361` |
| `test_confirm_is_one_shot_and_holds_the_write_lock_for_all_three_posts` | 1 | synthetic_runtime_test | `tests/test_mutations.py:399–472` | `TestParity_PythonTestTestsTestMutationsTestConfirmIsOneShotAndHoldsTheWriteLockForAllThreePostsL399` |
| `test_confirm_fails_closed_on_absolute_transition_and_otp_branch` | 1 | synthetic_runtime_test | `tests/test_mutations.py:475–515` | `TestParity_PythonTestTestsTestMutationsTestConfirmFailsClosedOnAbsoluteTransitionAndOtpBranchL475` |
| `test_confirm_requires_done_marker_and_post_response_save_failure_is_uncertain` | 1 | synthetic_runtime_test | `tests/test_mutations.py:518–559` | `TestParity_PythonTestTestsTestMutationsTestConfirmRequiresDoneMarkerAndPostResponseSaveFailureIsUncertainL518` |
| `test_cancellation_after_mutation_send_begins_is_reported_as_uncertain` | 1 | synthetic_runtime_test | `tests/test_mutations.py:562–587` | `TestParity_PythonTestTestsTestMutationsTestCancellationAfterMutationSendBeginsIsReportedAsUncertainL562` |
| `test_prepare_rejects_a_reused_draft` | 1 | synthetic_runtime_test | `tests/test_mutations.py:590–612` | `TestParity_PythonTestTestsTestMutationsTestPrepareRejectsAReusedDraftL590` |
| `test_parse_operation_details` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:87–94` | `TestParity_PythonTestTestsTestNewReadResourcesTestParseOperationDetailsL87` |
| `test_parse_card_info_keeps_only_last4_never_full_pan` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:97–109` | `TestParity_PythonTestTestsTestNewReadResourcesTestParseCardInfoKeepsOnlyLast4NeverFullPanL97` |
| `test_parse_pfm_amounts` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:112–121` | `TestParity_PythonTestTestsTestNewReadResourcesTestParsePfmAmountsL112` |
| `test_operation_details_sends_uohid` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:126–130` | `TestParity_PythonTestTestsTestNewReadResourcesTestOperationDetailsSendsUohidL126` |
| `test_operation_details_rejects_bad_id` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:133–137` | `TestParity_PythonTestTestsTestNewReadResourcesTestOperationDetailsRejectsBadIdL133` |
| `test_card_info_sends_string_ids_and_limits_helper` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:140–147` | `TestParity_PythonTestTestsTestNewReadResourcesTestCardInfoSendsStringIdsAndLimitsHelperL140` |
| `test_card_info_rejects_bad_ids` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:150–155` | `TestParity_PythonTestTestsTestNewReadResourcesTestCardInfoRejectsBadIdsL150` |
| `test_analytics_amounts_builds_request_and_parses` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:158–169` | `TestParity_PythonTestTestsTestNewReadResourcesTestAnalyticsAmountsBuildsRequestAndParsesL158` |
| `test_analytics_rejects_bad_args` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:172–177` | `TestParity_PythonTestTestsTestNewReadResourcesTestAnalyticsRejectsBadArgsL172` |
| `test_new_read_paths_are_in_allowlist` | 1 | synthetic_runtime_test | `tests/test_new_read_resources.py:180–181` | `TestParity_PythonTestTestsTestNewReadResourcesTestNewReadPathsAreInAllowlistL180` |
| `test_operation_amount_is_transaction_and_resource_billing_amount_is_balance_after` | 1 | synthetic_runtime_test | `tests/test_operation_semantics.py:7–37` | `TestParity_PythonTestTestsTestOperationSemanticsTestOperationAmountIsTransactionAndResourceBillingAmountIsBalanceAfterL7` |
| `test_plain_billing_amount_is_not_guessed_to_be_a_balance` | 1 | synthetic_runtime_test | `tests/test_operation_semantics.py:40–59` | `TestParity_PythonTestTestsTestOperationSemanticsTestPlainBillingAmountIsNotGuessedToBeABalanceL40` |
| `test_legacy_operation_constructor_keeps_its_positional_contract` | 1 | synthetic_runtime_test | `tests/test_operation_semantics.py:62–72` | `TestParity_PythonTestTestsTestOperationSemanticsTestLegacyOperationConstructorKeepsItsPositionalContractL62` |
| `test_parser_self_test` | 1 | synthetic_runtime_test | `tests/test_parser_selftest.py:1–6` | `TestParity_PythonTestTestsTestParserSelftestTestParserSelfTestL1` |
| `test_srp_matches_extracted_frontend_vector` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:248–265` | `TestParity_PythonTestTestsTestPinAuthTestSrpMatchesExtractedFrontendVectorL248` |
| `test_frontend_config_parser_is_dynamic_and_fails_closed` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:268–281` | `TestParity_PythonTestTestsTestPinAuthTestFrontendConfigParserIsDynamicAndFailsClosedL268` |
| `test_browser_cookie_import_requires_private_file` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:284–307` | `TestParity_PythonTestTestsTestPinAuthTestBrowserCookieImportRequiresPrivateFileL284` |
| `test_full_pin_login_rotates_csrf_follows_redirect_and_returns_ufs_session` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:310–333` | `TestParity_PythonTestTestsTestPinAuthTestFullPinLoginRotatesCsrfFollowsRedirectAndReturnsUfsSessionL310` |
| `test_pin_otp_retry_and_confirm_keep_one_auth_process` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:336–356` | `TestParity_PythonTestTestsTestPinAuthTestPinOtpRetryAndConfirmKeepOneAuthProcessL336` |
| `test_captcha_challenge_can_be_fetched_without_resending_pin` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:359–379` | `TestParity_PythonTestTestsTestPinAuthTestCaptchaChallengeCanBeFetchedWithoutResendingPinL359` |
| `test_wrong_pin_error_never_contains_submitted_pin` | 1 | synthetic_runtime_test | `tests/test_pin_auth.py:382–385` | `TestParity_PythonTestTestsTestPinAuthTestWrongPinErrorNeverContainsSubmittedPinL382` |
| `test_primary_auth_har_flow_enrolls_pin_and_returns_ufs_session` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:382–435` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryAuthHarFlowEnrollsPinAndReturnsUfsSessionL382` |
| `test_primary_wrong_sms_keeps_the_challenge_for_a_second_attempt` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:438–464` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryWrongSmsKeepsTheChallengeForASecondAttemptL438` |
| `test_primary_retry_otp_reports_exhausted_state` | 2 | synthetic_runtime_test | `tests/test_primary_auth.py:474–496` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryRetryOtpReportsExhaustedStateL474` |
| `test_primary_web_pin_skip_finishes_without_creating_pin` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:499–522` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryWebPinSkipFinishesWithoutCreatingPinL499` |
| `test_primary_captcha_retry_preserves_rotated_token` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:525–557` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryCaptchaRetryPreservesRotatedTokenL525` |
| `test_primary_malformed_srp_does_not_retain_password_in_traceback` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:573–593` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryMalformedSrpDoesNotRetainPasswordInTracebackL573` |
| `test_primary_malformed_rsa_key_does_not_retain_pin_in_traceback` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:596–618` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryMalformedRsaKeyDoesNotRetainPinInTracebackL596` |
| `test_primary_create_pin_key_survives_a_failed_attempt` | 1 | synthetic_runtime_test | `tests/test_primary_auth.py:621–643` | `TestParity_PythonTestTestsTestPrimaryAuthTestPrimaryCreatePinKeySurvivesAFailedAttemptL621` |
| `test_profile_401_reauthenticates_once_retries_and_atomically_saves` | 1 | synthetic_runtime_test | `tests/test_profile_reauth.py:155–203` | `TestParity_PythonTestTestsTestProfileReauthTestProfile401ReauthenticatesOnceRetriesAndAtomicallySavesL155` |
| `test_concurrent_expiry_uses_a_single_pin_login` | 1 | synthetic_runtime_test | `tests/test_profile_reauth.py:206–228` | `TestParity_PythonTestTestsTestProfileReauthTestConcurrentExpiryUsesASinglePinLoginL206` |
| `test_pin_profile_rejects_missing_deviceprint_before_first_request` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:185–201` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestPinProfileRejectsMissingDeviceprintBeforeFirstRequestL185` |
| `test_concurrent_persistent_401_does_not_repeat_pin_for_each_waiter` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:204–224` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestConcurrentPersistent401DoesNotRepeatPinForEachWaiterL204` |
| `test_waiter_arriving_after_swap_does_not_start_a_second_pin_login` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:227–253` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestWaiterArrivingAfterSwapDoesNotStartASecondPinLoginL227` |
| `test_close_racing_pin_login_does_not_leak_replacement_transport` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:256–275` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestCloseRacingPinLoginDoesNotLeakReplacementTransportL256` |
| `test_concurrent_close_closes_each_owned_transport_once` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:278–297` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestConcurrentCloseClosesEachOwnedTransportOnceL278` |
| `test_old_transport_close_failure_does_not_cancel_business_retry` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:300–317` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestOldTransportCloseFailureDoesNotCancelBusinessRetryL300` |
| `test_provider_error_keeps_profile_usable_for_a_later_retry` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:320–355` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestProviderErrorKeepsProfileUsableForALaterRetryL320` |
| `test_later_request_can_refresh_a_generation_rejected_by_the_retry` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:358–378` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestLaterRequestCanRefreshAGenerationRejectedByTheRetryL358` |
| `test_close_releases_the_pin_provider_reference` | 1 | synthetic_runtime_test | `tests/test_profile_reauth_edge_cases.py:381–392` | `TestParity_PythonTestTestsTestProfileReauthEdgeCasesTestCloseReleasesThePinProviderReferenceL381` |
| `test_v3_roundtrip_keeps_private_profile_and_full_cookie_metadata` | 1 | synthetic_runtime_test | `tests/test_profile_schema.py:14–72` | `TestParity_PythonTestTestsTestProfileSchemaTestV3RoundtripKeepsPrivateProfileAndFullCookieMetadataL14` |
| `test_empty_v3_profile_only_builds_non_ready_seed` | 1 | synthetic_runtime_test | `tests/test_profile_schema.py:75–84` | `TestParity_PythonTestTestsTestProfileSchemaTestEmptyV3ProfileOnlyBuildsNonReadySeedL75` |
| `test_v1_file_loads_and_is_saved_as_v3` | 1 | synthetic_runtime_test | `tests/test_profile_schema.py:87–130` | `TestParity_PythonTestTestsTestProfileSchemaTestV1FileLoadsAndIsSavedAsV3L87` |
| `test_v2_file_loads_and_is_saved_as_v3` | 1 | synthetic_runtime_test | `tests/test_profile_schema.py:133–146` | `TestParity_PythonTestTestsTestProfileSchemaTestV2FileLoadsAndIsSavedAsV3L133` |
| `test_antifraud_deviceprint_is_imported_from_private_har_without_logging_it` | 1 | synthetic_runtime_test | `tests/test_profile_schema.py:149–187` | `TestParity_PythonTestTestsTestProfileSchemaTestAntifraudDeviceprintIsImportedFromPrivateHarWithoutLoggingItL149` |
| `test_a5_one_malformed_optional_amount_does_not_discard_the_page` | 1 | synthetic_runtime_test | `tests/test_regressions.py:24–42` | `TestParity_PythonTestTestsTestRegressionsTestA5OneMalformedOptionalAmountDoesNotDiscardThePageL24` |
| `test_a5_malformed_core_amount_still_raises` | 1 | synthetic_runtime_test | `tests/test_regressions.py:45–50` | `TestParity_PythonTestTestsTestRegressionsTestA5MalformedCoreAmountStillRaisesL45` |
| `test_a8_non_list_product_data_raises_parseerror_not_typeerror` | 1 | synthetic_runtime_test | `tests/test_regressions.py:53–57` | `TestParity_PythonTestTestsTestRegressionsTestA8NonListProductDataRaisesParseerrorNotTypeerrorL53` |
| `test_a11_mixed_format_dates_sort_without_error_and_in_order` | 1 | synthetic_runtime_test | `tests/test_regressions.py:60–67` | `TestParity_PythonTestTestsTestRegressionsTestA11MixedFormatDatesSortWithoutErrorAndInOrderL60` |
| `test_a12_valueless_cookie_becomes_empty_string` | 1 | synthetic_runtime_test | `tests/test_regressions.py:70–77` | `TestParity_PythonTestTestsTestRegressionsTestA12ValuelessCookieBecomesEmptyStringL70` |
| `test_a13_transfer_resource_name_redacts_embedded_pan` | 1 | synthetic_runtime_test | `tests/test_regressions.py:80–88` | `TestParity_PythonTestTestsTestRegressionsTestA13TransferResourceNameRedactsEmbeddedPanL80` |
| `test_a20_currency_null_falls_back_and_list_is_not_stringified` | 1 | synthetic_runtime_test | `tests/test_regressions.py:91–94` | `TestParity_PythonTestTestsTestRegressionsTestA20CurrencyNullFallsBackAndListIsNotStringifiedL91` |
| `test_api_base_is_extracted_from_runtime_main_config` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:71–80` | `TestParity_PythonTestTestsTestSberClientTestApiBaseIsExtractedFromRuntimeMainConfigL71` |
| `test_runtime_hosts_fail_closed_on_conflicts_or_non_origins` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:83–96` | `TestParity_PythonTestTestsTestSberClientTestRuntimeHostsFailClosedOnConflictsOrNonOriginsL83` |
| `test_sensitive_har_seed` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:99–123` | `TestParity_PythonTestTestsTestSberClientTestSensitiveHarSeedL99` |
| `test_sanitized_har_is_rejected` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:126–138` | `TestParity_PythonTestTestsTestSberClientTestSanitizedHarIsRejectedL126` |
| `test_products_request_shape` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:141–160` | `TestParity_PythonTestTestsTestSberClientTestProductsRequestShapeL141` |
| `test_observed_user_agent_and_client_hints_are_preserved` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:163–179` | `TestParity_PythonTestTestsTestSberClientTestObservedUserAgentAndClientHintsArePreservedL163` |
| `test_operations_n_plus_one_and_dates` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:182–215` | `TestParity_PythonTestTestsTestSberClientTestOperationsNPlusOneAndDatesL182` |
| `test_redirect_means_expired_session` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:218–227` | `TestParity_PythonTestTestsTestSberClientTestRedirectMeansExpiredSessionL218` |
| `test_warmup_accepts_empty_204` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:230–239` | `TestParity_PythonTestTestsTestSberClientTestWarmupAcceptsEmpty204L230` |
| `test_stale_session_is_warmed_before_business_request` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:242–264` | `TestParity_PythonTestTestsTestSberClientTestStaleSessionIsWarmedBeforeBusinessRequestL242` |
| `test_set_cookie_domain_without_dot_is_domain_cookie` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:267–274` | `TestParity_PythonTestTestsTestSberClientTestSetCookieDomainWithoutDotIsDomainCookieL267` |
| `test_set_cookie_uses_rfc_default_path` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:277–279` | `TestParity_PythonTestTestsTestSberClientTestSetCookieUsesRfcDefaultPathL277` |
| `test_later_set_cookie_deletes_prior_cookie` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:282–310` | `TestParity_PythonTestTestsTestSberClientTestLaterSetCookieDeletesPriorCookieL282` |
| `test_cookie_domain_leading_dot_is_canonicalized` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:313–340` | `TestParity_PythonTestTestsTestSberClientTestCookieDomainLeadingDotIsCanonicalizedL313` |
| `test_set_cookie_rejects_sibling_domain` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:343–349` | `TestParity_PythonTestTestsTestSberClientTestSetCookieRejectsSiblingDomainL343` |
| `test_operations_freezes_default_from_between_pages` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:352–382` | `TestParity_PythonTestTestsTestSberClientTestOperationsFreezesDefaultFromBetweenPagesL352` |
| `test_netscape_cookie_file_preserves_metadata_and_skips_expired` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:385–399` | `TestParity_PythonTestTestsTestSberClientTestNetscapeCookieFilePreservesMetadataAndSkipsExpiredL385` |
| `test_insecure_cookie_file_is_rejected` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:402–407` | `TestParity_PythonTestTestsTestSberClientTestInsecureCookieFileIsRejectedL402` |
| `test_external_cookie_file_completes_sanitized_har` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:410–427` | `TestParity_PythonTestTestsTestSberClientTestExternalCookieFileCompletesSanitizedHarL410` |
| `test_cookie_must_apply_to_both_session_hosts` | 2 | synthetic_runtime_test | `tests/test_sber_client.py:431–447` | `TestParity_PythonTestTestsTestSberClientTestCookieMustApplyToBothSessionHostsL431` |
| `test_transport_ignores_environment_ca_bundles` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:450–472` | `TestParity_PythonTestTestsTestSberClientTestTransportIgnoresEnvironmentCaBundlesL450` |
| `test_impersonation_follows_client_hints_before_overridden_ua` | 1 | synthetic_runtime_test | `tests/test_sber_client.py:475–480` | `TestParity_PythonTestTestsTestSberClientTestImpersonationFollowsClientHintsBeforeOverriddenUaL475` |
| `test_curl_response_honors_authoritative_samesite_without_losing_untouched_policy` | 13 | synthetic_runtime_test | `tests/test_transport_cookie_metadata.py:56–98` | `TestParity_PythonTestTestsTestTransportCookieMetadataTestCurlResponseHonorsAuthoritativeSamesiteWithoutLosingUntouchedPolicyL56` |
| `test_curl_response_retains_mixed_case_previous_policy` | 3 | synthetic_runtime_test | `tests/test_transport_cookie_metadata.py:102–125` | `TestParity_PythonTestTestsTestTransportCookieMetadataTestCurlResponseRetainsMixedCasePreviousPolicyL102` |
| `test_curl_response_rejects_mixed_case_invalid_policy` | 3 | synthetic_runtime_test | `tests/test_transport_cookie_metadata.py:129–146` | `TestParity_PythonTestTestsTestTransportCookieMetadataTestCurlResponseRejectsMixedCaseInvalidPolicyL129` |
| `test_metadata_reconciliation_does_not_delete_another_requests_cookie` | 1 | synthetic_runtime_test | `tests/test_transport_cookie_metadata.py:149–167` | `TestParity_PythonTestTestsTestTransportCookieMetadataTestMetadataReconciliationDoesNotDeleteAnotherRequestsCookieL149` |

## Reusable offline inventory validation

From the Go repo, with the audited Python reference still present at the manifest path:

```sh
python testdata/compat/verify_inventory.py
```

This development-only checker parses AST/JSON, compares exact sets, validates every source/fixture digest and provenance line range, checks schema safety and recomputes totals/parameter cases. It never imports the bank transport, runs bank authentication or changes repository state. It is **not** a Go runtime/test dependency and does not verify Go implementation. The observed offline SRP vector/config/negative-RSA/parser materialization checks are recorded separately in JSON.

## Native implementation completion gate

- Each row has actual native implementation and exercised tests, including explicit adaptation of unsafe/legacy/reference gaps.
- Native unit/contract/fuzz/race/build evidence is required; no Python subprocess shim or placeholder counts.
- Public browser→Go HTTP continuity and owner-operated auth/complete-history checks are separate evidence levels. No fixture pass proves those live levels.
- No messages to tenants and no autonomous bank mutation are authorized by this matrix.
