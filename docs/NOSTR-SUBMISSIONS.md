# Automatic submissions, OpenCode and Nostr

The static Astro site stays on GitHub Pages. The Go service owns durable intake, SQLite/WAL,
OpenCode jobs, public activity, Nostr delivery and automatic report publication.

The user requested automatic processing without human approval gates. Trustworthy, sourced
in-scope reports are validated and committed directly to `REPO_PATH`. Open questions and minor
source disagreements are preserved in the report's `open_questions` front matter and displayed
on its page. Reject unsupported events, scams/misinformation and untrustworthy material; do not
reject a supported incident merely because details are missing or differ between outlets.
The service does not open PRs, call GitHub to publish, push commits or deploy the website.
The submissions overview shows committed articles per minute as a rolling 60-minute average.
It counts recorded local publications once per case, across all origins and list filters;
duplicate receipts and unrelated status updates do not increase the rate.

## Local development

Run these in separate terminals from the repository root:

```sh
npm run backend-dev
npm run dev
```

`backend-dev` loads the ignored root `.env` and starts Go on `127.0.0.1:8080`. It defaults to
the locally installed OpenCode CLI, automatic publication, and private state under
`backend/data/`. Set `RESEARCH_MODEL` in `.env` (or the process environment) to choose the
provider/model, including `#effort` when the reasoning effort is known (for example `#xhigh`). There is no built-in model default;
processing stays paused until a model is configured. Astro proxies HTTP and WebSocket traffic through
`/__submissions`, so the frontend also works when port 4321 is occupied and Astro selects 4322.
Production builds require an explicit `PUBLIC_SUBMISSIONS_API_URL`.

Private local `.env` settings:

```dotenv
RESEARCH_RUNNER=local
RESEARCH_MODEL=opencode-go/deepseek-v4.1-flash
AUTO_PUBLISH=true
```

Keep `.env` mode 0600 and out of Git. Never prefix a secret with `PUBLIC_`.
The local runner uses OpenCode’s saved credentials. It also loads the selected provider’s
endpoint, model definitions and options from `~/.config/opencode/opencode.json` or
`opencode.jsonc` (under `$XDG_CONFIG_HOME` when set), or the file named by `OPENCODE_CONFIG`.
JSONC comments and trailing commas are supported. The legacy `~/.opencode/opencode.json(c)`
location remains supported; standard global settings override it, and JSONC overrides matching
JSON settings while retaining other provider/model settings. OpenCode 1's `provider`/`options`
and OpenCode 2's `providers`/`settings` definitions are imported in their native format,
including custom runtime packages and model reasoning variants. Only that provider is imported
into the isolated session; home agents, plugins, permissions and unrelated provider credentials
are not imported. A provider key from `options.apiKey` (v1), `settings.apiKey` (v2), a
`{env:NAME}` reference, or the selected provider's `env` list is passed privately to the harness and redacted from
startup diagnostics and public activity. The provider config stays outside the evidence input.
`RESEARCH_MODEL` still selects the provider/model; restart the backend after changing either.
OpenCode 2 uses its native scoped
`auth export`/`auth import` commands to initialize a private standalone session. It imports the
selected model integration and the search integration chosen by `RESEARCH_WEBSEARCH` (default
`exa`), including credentials saved through `/connect`. With `RESEARCH_WEBSEARCH=random`, it
imports only the supported search integrations and lets OpenCode switch providers on HTTP 429.
Unconnected search services retain OpenCode's normal anonymous/availability behavior; a saved
connection does not remove the provider's account limits. Credential contents never enter logs
or `.env`. Search credentials are imported even when the model uses an inline provider key.
OpenCode 1 copies only the selected provider from its
legacy auth file (`OPENCODE_AUTH_FILE` can override that file). Local mode does not require
`RESEARCH_API_KEY`. Docker deployments can provide a dedicated provider key.
On every backend start, the service checks the binary/version and runs a short, no-tool
`opencode run` prompt asking the selected model to reply `OK`. Research starts only after that
test passes. A successful non-empty assistant reply establishes connectivity; the requested
wording is not an exact-output gate. An empty successful stop is retried once, with both
attempts retained in the diagnostics. The probe has a two-step allowance to avoid OpenCode’s
forced last-step summary on the first request. The check is bounded to 60 seconds and appears live in the UI. A failed check leaves
intake/status endpoints running and jobs queued; authenticate the provider in OpenCode and restart.
You can run the same check without starting workers using `cd backend && go run ./cmd/tracker -check-ai`
(with the backend environment loaded). The startup log includes the exact user prompt, configured
agent instruction, model/CLI arguments, parsed response, exit code, duration and stderr. Failures
also include a bounded stdout preview. Full captured stdout/stderr are saved, with credential
redaction, to `WORKSPACE_PATH/startup-check.json` (default `backend/data/jobs/startup-check.json`
from the repository root), mode 0600. This file is replaced on each check. These operator
diagnostics are excluded from public health/config JSON.
Restart `backend-dev` after changing environment settings or saved auth. `/v1/config` and the public request
register explicitly report the binary/model startup check and any unavailable research configuration.

The harness is **OpenCode**, not a replacement direct-LLM client. The local runner detects the
installed version and uses native OpenCode 2 configuration with `run --standalone`, or legacy
OpenCode 1 configuration with `run --pure`. Both select the researcher agent, configured model
and JSON stream in a disposable read-only evidence workspace. A functional startup prompt,
rather than an exact local version number, decides readiness. The optional Docker image pins
OpenCode 1.17.13 for reproducible deployment.
OpenCode's researcher agent and editorial skills drive source research and the final structured
artifact. The service independently checks source access, quotations, output schema and builds.

The local macOS runner uses `sandbox-exec`, a separate HOME/XDG/TMP directory, and filesystem rules
that deny host home directories, other temporary data and service data while allowing the job's
input and private session storage. It never inherits production/Nostr/GitHub environment variables.
Agent bash, edits, external directories and task delegation are denied, except for the exact
isolated session's `data/opencode/tool-output/*` directory. OpenCode saves oversized tool results
there; the researcher may read/search those generated files without an external-directory prompt.
The same narrow exception uses `/tmp/data/opencode/tool-output/*` in the Docker runner. It does
not include the rest of session storage, auth files, other jobs or host directories. This local development
sandbox is a filesystem boundary, not a production network firewall. Use the Docker runner with
enforced egress controls for unattended deployment. Local OpenCode mode on other platforms fails explicitly
rather than silently running without its sandbox.

Go 1.27.3 is pinned in `backend/go.mod`. Backend checks/build:

```sh
cd backend
go test -race ./...
go vet ./...
go build -o tracker ./cmd/tracker
```

## Public pages

- `/submit/`: compatibility redirect to `/submissions/`.
- `/submission/?id=…`: public status, history, model/session information and live AI activity.
- `/submissions/`: URL submission form at the top, optional public notes, folded browser receipts
  with JSON import/export, and all request origins and statuses in a 50-row table with filters,
  live OpenCode chat/activity, and worker configuration status. The existing Submit menu link points to the form on this page.

Both AI views offer **Show full log (JSON)** and **Download JSON**, containing every recorded
public activity event in chronological order and all run metadata, including retries. The JSON
view refreshes on live updates while open. `GET /v1/submissions/{id}/ai/log` returns the same
versioned export without the normal chat view's 100-message/10-run limits. Events omitted by
the public activity recorder (including raw tool output) are not reconstructed by this export.

`backend-dev` also enables `RAW_AI_LOGS=true`. In the JSON view, select **Raw output + emitted
reasoning** to inspect every captured OpenCode event (including reasoning, tool arguments,
results, final artifacts and errors) and stderr per run. The raw view refreshes every two seconds
while open, and its download includes the same data. `GET /v1/submissions/{id}/ai/raw` serves it.
Raw capture is written during execution; older completed runs have their original event archive
but may lack stderr. Reasoning is available only when emitted by OpenCode/the provider. Configured
credentials and credential fields are redacted. This diagnostic endpoint is off by default outside
`backend-dev`; enabling it makes raw logs accessible to anyone who can access that backend and
receipt, so keep the development backend on localhost.

The AI view shows the selected model/provider, OpenCode session ID, run status, reported token
usage and a bounded public transcript of request messages, assistant updates and tool activity.
It does not expose hidden reasoning, tool arguments/output, article bodies, raw final JSON,
credentials or private workspace paths. Full execution/evidence files stay private. OpenCode text
updates stream as the CLI produces them; the structured final artifact remains private and its
sanitized public explanation appears separately. Recent attempts remain visible after retries.

“Queued” means intake is committed, not that a model ran. Without working OpenCode authentication/model access, the worker keeps jobs durable and the
UI shows the startup test failure. Research can finish before
the publication job runs: a separate durable job commits it to the configured local repository.
No GitHub App or API key is needed for this step.

## Browser source capture

Install the matching Chromium once with `npm run browser-install` (Playwright is a pinned runtime
dependency). `SOURCE_BROWSER=true` is the default. Initial and corroborating sources load in fresh,
anonymous Chromium contexts; no saved browser profile, cookies, service keys or account is loaded.
JavaScript and public API requests are supported. Chromium makes its own connections (its own TLS,
HTTP/2 and headers, with a normal Chrome user agent, so sites behind bot checks see a browser rather
than a Go client) through a per-capture, password-protected loopback proxy: HTTPS tunnels to port
443 and plain HTTP to port 80 only, to hosts whose every address is public, dialled at the checked
address (no DNS rebinding). Loopback is not exempt from the proxy. Ads and trackers are still blocked
by the filter lists; service workers, WebSockets, downloads and media streaming are blocked.
Redirects re-enter the same destination checks. Captures have connection, byte and time limits.

The archive under `WORKSPACE_PATH/sources/<case>/<capture>/` contains `page.html` (rendered markup),
`page.txt` (visible text), `page.png` and `metadata.json`, mode 0600 inside private directories.
Screenshots are at most 1440×12000 pixels; a warning records clipping or screenshot failure.
Metadata includes title/description, canonical URL, language, keywords, author/date/publisher
hints, all bounded meta tags, Open Graph, Twitter cards and parsed JSON-LD. These are provenance
hints, never a substitute for opened article text. OpenCode receives visible text, rendered markup,
metadata and the optional screenshot; independent quotation checks use the rendered visible text.

The local request detail page shows selected metadata expanded and an inline screenshot preview,
which links to the full-size capture.
Report previews use the prepared report file's selected media, including an empty selection;
captured research photos remain in the source-check section. Once a submission is committed,
published or matched to an existing report, its inline preview loads the actual report article,
and its full preview opens that report page. Covers, photos and later edits therefore stay in sync.
Screenshots use a loopback-only development endpoint, not the public static report assets; no
third-party page images are copied into published reports. A login, paywall or access challenge
can still prevent readable evidence. If Chromium is missing, static HTTP retrieval remains
available with an explicit browser warning; JS-only sources need the browser installed.

The intake-only container does not contain this Node/browser runtime. Full research/publishing
hosts need the repository dependencies and matching Chromium installed (plus Chromium's Linux
system dependencies). Keep capture directories under the same private retention/disk quotas as
research evidence. `SOURCE_BROWSER=false` explicitly opts out of browser capture.

## Processing and publication

Progress is:

`queued → fetching → classifying → researching → drafted → publishing → committed`

`committed` means that the cited report is in a local Git commit and available to the local Astro
server. It does not claim that a remote website has deployed. Existing `published` receipts keep
their previous meaning. The UI links to the report for either successful state.

Other outcomes: already covered (`duplicate`), outside scope (`not_relevant`), unsupported or
untrustworthy (`insufficient_evidence`), routed to CFI (`routed_to_cfi`), inaccessible source
(`source_unavailable`) and technical/schema/build errors (`failed`). Minor discrepancies such as
vegetable type or a missing court outcome belong in `open_questions`, not a failed outcome.
Every factual claim still needs a quotation independently confirmed in an opened source.
Rejection/outcome messages use an optional short `decision_title` and a fuller public
`explanation` (up to 1,500 characters), with relevant opened articles in `sources`. The request
page shows the short heading, detailed reason and source/report links. Backend validation stops
show the service's reason separately from the saved model assessment. Source-overlap stops also
record the shared article and related public submission/report, rather than imply the model
rejected the event. Older results fall back to the status label and their saved explanation;
no longer model explanation is fabricated for historical attempts.

Automated report status remains `reported`, never automatically `verified`.
Each newly collected photo or video requires its own media entry with `url` (the cited source page),
`media_url` (the direct absolute HTTP(S) photo/video address), publisher and caption/credit. OpenCode
copies the address from captured `source_images[].src` or the opened page, never guesses it.
The service validates and persists this field; it does not download or embed the photo.
Legacy report media may omit it. Multiple photos on one source page remain separate entries.
The service must write `created_by_model` from the actual run configuration, including the provider,
model and reasoning effort whenever available: `provider/model#effort`, for example
`alibaba-token-plan/qwen3.8-flash#xhigh`. The `xhigh` suffix is the effort setting, also called
a reasoning variant. Include the actual effort even when it is configured separately from the
model identifier; never infer it from current defaults or silently omit a known setting.
If effort is unavailable, keep the known `provider/model` without a suffix. It is not a
model-authored field.
Existing-case updates retain the original creator and do not record update-model metadata.
Creator attribution appears on the report detail page and in the report JSON API. `created_by_model` is required on every report. Historical reports without a known creator use
`unknown` and display "not recorded"; never infer their model from today's configuration.

Repeated normalized URLs return the first existing submission ID, including across website,
Nostr and discovery intake and after terminal outcomes. Tracking parameters and fragments removed
by URL normalization do not create another receipt. Resubmitting a failed URL does not restart it;
use the existing loopback restart control. When the AI research finished before the failure, the
page also offers "Retry from last step" (`POST /v1/submissions/{id}/restart` with `{"resume": true}`):
it keeps the saved research and redoes the source checks, build and commit without a new AI run.
"Restart from scratch" drops the saved research. A draft that fails the build is built again without
the draft; when the repository fails too, the request says so instead of blaming the draft. Historical duplicate receipts remain readable, with
all their notes/history intact; future submissions reuse the earliest one. Existing-case
updates preserve the stable slug, earlier prose/citations and existing open questions, adding
new questions without duplication. Unsupported event identity or ambiguous duplicate matches
still need explicit handling rather than fabricated certainty.

The validation checkout overlays the current trusted site files, editorial rules and reports,
including development edits, without copying `.env` or backend state. The site is not built:
each report file the run writes is checked on its own (`CheckReport` in
`backend/internal/research/report_check.go`) against the content schema's rules
(`src/content.config.ts`) and the report tests (`scripts/reports.test.mjs`): known fields and types,
country, type, precision and status ids, its reports/<year>/<month>/ path and slug, and every
citation naming a source and every source cited. A test runs the check over every existing report,
so it cannot be stricter than the site. The commit accepts only the validated report and
its old dated path when moving an existing report. It refuses local edits on those paths and
concurrent changes to an existing report. Git `commit --only` excludes unrelated staged files.
A case trailer and a persisted SHA recover publication retries without a second commit.

On localhost, **Restart submission** appears for `budget_wait`, `failed`, `insufficient_evidence`,
`source_unavailable` and legacy `needs_review` receipts on their request detail page only.
`backend-dev` sets `DEV_CONTROLS=true`; the backend must also bind to a loopback address and
receive the request from loopback with a local origin and JSON content type. The server refuses
active/successful requests and duplicate restarts. Restarting resets the research job's retry
counter, removes a stale publication job and queues one new attempt for the shared case. Previous
history, AI runs and messages remain available. Daily AI budget reservations are retained.

## API

| Method | Route | Behavior |
|---|---|---|
| POST | `/v1/submissions` | `{url,note?}` → durable `202 {id,status,status_url,nostr_event_id:null}` |
| GET | `/v1/submissions/{id}` | Public source, note, status, explanation, case/history/links |
| GET | `/v1/submissions/{id}/ai` | Processing configuration, 10 recent runs, latest 100 public activity messages |
| GET | `/v1/submissions/{id}/ai?after=n` | Next bounded activity batch after its numeric message ID |
| GET | `/v1/submissions?ids=a,b` | Up to 50 browser receipts, missing IDs omitted |
| GET | `/v1/submissions?cursor=n&status=…&origin=…` | All origins, 50 per page, total, next cursor, revision and processing status |
| WebSocket | `/v1/submissions/stream` | `{type:"submissions_changed",revision:"…"}` invalidations after public changes |
| GET | `/v1/activity?cursor=n` | Up to 25 terminal human-submission outcomes |
| POST | `/v1/submissions/{id}/restart` | Local development only; requeue a failed shared case, preserving history |
| GET | `/v1/stats/ai?period=all` (also `30d`, `today`) | Aggregate research runs and token/cost telemetry by model and pricing tier; no individual run identifiers |
| GET | `/v1/config` | Public Nostr and AI processing configuration, never credentials |
| GET | `/healthz` | Database liveness |

IDs are 32-character random hex references; timestamps are Unix seconds. A receipt is public,
not a password or deletion credential. CORS accepts exact configured origins without cookies.
Public projections exclude private job provenance, evidence, model prompts and host paths.

Service-only endpoints:

- `POST /v1/ingest/candidate`: Bearer `INGEST_TOKEN`; JSON `{url,origin,provenance,idempotency_key}`.
- `GET /internal/metrics`: same token; bounded queue/running/dead counts.
- The legacy GitHub webhook is not used by the local commit pipeline.

## Live updates

Forward HTTP/1.1 WebSocket Upgrade/Connection headers for `/v1/submissions/stream`, with proxy idle
timeouts exceeding the 25-second heartbeat. HTTPS API URLs become WSS automatically. Connections
have exact origin checks, a 128-connection cap and 30-minute lifetime. Each connection checks one
SQLite version row per second. Transactional triggers cover intake, shared-case transitions,
Nostr links, model/run usage and public AI messages. A notification invalidates only the current
bounded table page and expanded details, not the entire corpus.

Clients reconnect with backoff, preserve the table on disconnection and pause while hidden.
The status page also subscribes so model messages appear without waiting for its fallback poll.

## Jobs, budgets and operations

The register's top counters cover the whole database, independent of filters/pagination:
all submission receipts, actively leased processing cases, queued cases and budget-paused cases.
New intake reuses an existing submission for the same normalized URL. Historical duplicate
receipts still share one processing case. Counters update over the same WebSocket.
A separate Working now table always shows active cases (one receipt per case, bounded to 50),
independent of history filters or pages. Restarted older submissions appear there when claimed.

The backend holds an exclusive OS lock beside its SQLite file. A second backend or repair command
cannot change the same queue concurrently. On startup, it immediately fences/requeues interrupted
jobs and Nostr deliveries, refunds the interrupted queue attempt, marks unfinished AI runs
`interrupted`, and repairs intermediate cases missing a research job. Explicit terminal outcomes,
dead jobs, scheduled budget waits, evidence, usage and reservations are retained. Graceful shutdown
also releases the active processing lease without spending a retry.

After a successful model response, a private SQLite checkpoint saves the result, original model
and initial evidence. Recovery creates a fresh trusted checkout, rechecks sources and rebuilds
without calling the model or reserving another AI run. If the server stopped before this checkpoint,
research restarts within the existing daily budget; an interrupted model call may already have
incurred provider cost. Manual Restart deliberately discards the checkpoint for a new attempt.
A queued publication always resumes its validated draft. Git case trailers detect a commit made
just before a crash, so recovery records its SHA/status without committing again.

SQLite uses WAL, foreign keys, busy timeout, atomic lease claims and fencing tokens. Research
heartbeats every 30 seconds on a two-minute lease. Technical failures back off and dead-letter
after eight attempts; configuration waits and daily budget waits do not consume attempts.
`RESEARCH_TIMEOUT_SECONDS` is 10–1800 (default 900). One research worker runs at a time.

A reservation limit pauses the case as `budget_wait`, with `resume_at` showing the job's next
scheduled attempt (00:01 UTC after the daily reset). Older receipts incorrectly left at
`classifying` are repaired by migration 008. No new AI run is started or charged while paused;
the previous run is identified as a previous attempt on the detail page. The conservative
reservation total is distinct from the estimated provider cost displayed from actual tokens.
The AI worker box shows today’s reserved amount against the configured daily limit, the per-run
reservation, remaining attempts and the next reset time in the browser’s local timezone. It uses
the same UTC-day reservation query as the budget guard and refreshes at least every 30 seconds.
The register uses one-line rows with stage segments, cumulative input/output tokens and run
counts across all attempts; its SQL projections remain bounded to the current 50-row page.

`RESEARCH_RUN_CENTS` reserves a conservative amount for a run, and `RESEARCH_DAILY_CENTS` limits
starts per UTC day. Set `RESEARCH_DAILY_CENTS=0` for an unlimited daily reservation budget.
Usage and per-run reservations are still recorded; the UI shows Unlimited rather than a zero
balance. The budget API sets `unlimited: true`, `daily_limit_cents: 0` and uses `-1` for
`remaining_cents` and `runs_remaining` in this mode. Restart the backend after changing this value. With unlimited mode enabled, startup
requeues existing budget-paused cases immediately, preserving history, evidence and usage.
Budget-paused submissions can also be restarted from their localhost detail page; this does not
bypass a finite daily limit.
These are run reservations, not a guarantee of metered model billing. Failed
runs count. OpenCode-reported token/cost telemetry is stored when available; enforce a provider
spending limit separately. Set filesystem quotas and retention for private evidence, session
storage, checkouts and build caches. The local runner has a wall-clock/output limit; Docker also
bounds CPU, memory, processes and scratch space.

The API caps bodies at 8 KiB, notes at 500 characters and outstanding jobs at 1,000. Source fetches
check HTTP(S) schemes, standard ports, redirects and DNS/IP destinations at connection time;
private/special addresses, oversized bodies and unsupported source formats are rejected. The
application's hardened fetcher does not make a local OpenCode tool a network sandbox: production
research needs an egress firewall/proxy denying private, host, loopback and metadata destinations.
Article content is untrusted data and may never grant tools or signing/publishing authority.

Short-lived rate limits cover IPs, authors and news hosts. Local web intake bypasses the per-IP
and news-host hourly limits when `DEV_CONTROLS=true`, the backend binds to loopback, and the
request arrives from loopback with a local or absent Origin (including Astro's development proxy).
Public intake retains both limits. URL/body validation and the durable queue capacity still apply. Behind a proxy, configure only actual
`TRUSTED_PROXY_CIDRS` and correct forwarding headers. Use upstream connection limits and minimize
proxy access-log retention. No long-term IPs are stored by the service. Accountless does not mean
network anonymous. Public URLs/notes/outcomes may persist on third-party Nostr relays forever.
This is not a confidential tip line.

Repair commands, with workers stopped:

```sh
./tracker -retry RECEIPT_ID
./tracker -resolve RECEIPT_ID -outcome not_relevant -explanation 'This is general policy news rather than one incident.'
./tracker -backup /private/backups/tracker-2026-10-29.sqlite
```

Online backup uses `VACUUM INTO`, reserves a new mode-0600 destination and refuses overwrite.
Restore with workers stopped, then verify health, known receipts and recovered leases before
resuming. Tests cover backup/restore. Keep encrypted database and evidence backups off-host.
Operator resolution commands remain optional repair tools, not approval gates.

## Docker deployment

Set `RESEARCH_RUNNER=docker`, a digest-pinned `RESEARCH_IMAGE`, restricted `RESEARCH_NETWORK`,
model and provider key. Build `backend/research-image/` with OpenCode 1.17.13. The container gets
only the explicit read-only research snapshot and provider key, never `.git`, SQLite, Nostr or
GitHub credentials. Use a dedicated non-root host/service, persistent private disk, TLS proxy,
secret environment configuration, restart management and tested backups. The intake-only
`backend/Dockerfile` does not include Git, Node/npm or Docker orchestration tools.

A Docker network name alone is not a firewall. Provision enforced public-only research egress
before enabling unattended production research. No external OpenCode server is needed: the
service invokes the actual CLI harness per run and parses its event stream and final artifact.

## Nostr and discovery

Separate gateway and bot identities handle accountless web notes and native kind-1 submissions.
Generate protected keys with `./tracker -keygen /secure/path/nostr.env`, and configure at least
two distinct WSS relays. Publish the startup bot npub as `PUBLIC_NOSTR_BOT_NPUB`. Native notes
must p-tag the bot and contain one news URL; signatures/IDs/age/rate limits are verified and
bot/gateway loops ignored. NIP-10 root/reply threading, NIP-65 metadata/recipient read relays
and NIP-19 npub links are supported. Signed events, targets and ACKs persist before delivery;
retries reuse the same event ID. Relay ACKs do not guarantee client visibility.

Back up keys encrypted off-host. On compromise, stop delivery, rotate/announce the replacement
through trusted channels and inspect pending signed events before resuming. Old relay events
cannot be recalled globally. Metadata/cursors backfill a five-minute overlap with a seven-day
maximum intake age; longer outages need an explicit recovery run.

Google News discovery is enabled by default. It runs shortly after backend startup and then
15 minutes after each completed scan, independently of RSS/GDELT collectors. Configure it with:

```dotenv
HARVEST_GOOGLE_NEWS_ENABLED=true
HARVEST_GOOGLE_NEWS_INTERVAL=15m
HARVEST_GOOGLE_NEWS_MAX_ARTICLES=25
HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN=12
# Optional pipe-separated queries; blank uses the built-in multilingual set. Prefix a query
# with its Google News edition (ceid, country:language); without one, US English is used.
HARVEST_GOOGLE_NEWS_QUERIES=[IN:hi] (गांजा OR चरस) (जब्त OR गिरफ्तार) when:1d|[CZ:cs] konopí policie when:1d
LOG_LEVEL=debug
```

The built-in set (`DefaultGoogleNewsQueries` in `backend/internal/harvest/google_news.go`) has
50 queries. It searches India in English, Hindi, Bengali, Marathi, Tamil, Telugu, Malayalam,
Gujarati and Punjabi; Bangladesh in Bengali; Pakistan in English; Kenya, Nigeria and southern
Africa in English, including regional terms such as bhang, dagga, mbanje and matekoane; Morocco
and France in French; Arabic in the Lebanese, Egyptian and Saudi editions; and Israel in Hebrew.
Asian editions include Indonesia, the Philippines, Thailand, Singapore, Malaysia, Vietnam,
mainland China, Hong Kong, Taiwan, Japan and South Korea. European/American editions include
Czech, German, Italian, Dutch, Polish, Romanian, Greek, Ukrainian, Swedish, Spanish, Russian,
Turkish, Brazilian Portuguese and Latin American Spanish, alongside US English and the Pacific.

Queries pair cannabis names, resin/oil forms, local synonyms and common inflections with seizure,
arrest, raid, cultivation/eradication, prosecution, conviction and punishment terms where practical.
Execution and death terms supplement seizure terms in English and relevant Asian/Arabic queries.
No synthetic-cannabinoid terms are added as cannabis synonyms. Mixed articles still require AI
research to establish a separately supported cannabis enforcement component; whole-article negative
keywords would also hide qualifying mixed events.

Edition support is checked through live RSS responses, not inferred from a language code. Gujarati
and Punjabi use Google's `gu-IN`/`pa-IN` UI locales. Kannada and Urdu probes redirected to Hindi and
English respectively, so no native-language edition is assumed for them. Nepal, Sri Lanka, Bhutan
and Pakistan have explicit English searches (plus best-effort Nepali terms in the Hindi edition).
A further English query names Iran, Afghanistan, North Korea, Myanmar and Cambodia rather than
assuming working Persian, Pashto, Burmese, Khmer or North Korean editions. Regional editions are
search locales, not strict event-country filters, and do not guarantee coverage of every country.

Each scan runs the next `HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN` queries in rotation (1–50), paced a
second apart. With 50 default queries and 12 per scan, five scans visit the whole set, roughly
75 minutes plus scan time (under two hours at the default interval and five-minute scan deadline),
inside the one-day lookback. The larger set does not increase the per-scan request or admission
budgets. New candidates are taken round-robin across that scan's queries, so a busy edition cannot
use up the per-scan budget of 1–100 new URLs (default 25). At most 50 configured queries, each at
most 1000 characters. Custom queries replace the built-in set. This is candidate discovery, not
exhaustive worldwide coverage or a relevance verdict: AI research decides from opened articles
which ones become reports. Each queued candidate records its edition and query in its provenance.
Restart the backend after changing the defaults or configuration.

An opt-in feed check tests every default query, verifies that Google retained the requested edition
rather than silently redirecting to another language, and logs result counts without resolving
articles, queuing URLs or starting AI research:
`cd backend && GOOGLE_NEWS_LIVE=1 go test ./internal/harvest -run TestGoogleNewsLiveFeeds -v`.

Google wrapper URLs are resolved to publisher article URLs before intake, using Google's public
article-page redirect RPC. That RPC is undocumented and may change. Unresolved candidates are
logged and retried on the next scan; aggregator pages are never submitted as article evidence.
All network calls use the existing public-destination-pinned fetch transport, bounded response
sizes and timeouts. Resolution attempts are paced and each scan has a five-minute deadline.
Successful discovery mappings persist in SQLite. Previously known canonical/source URLs and
repeated Google results are skipped; queue admission retains durable idempotency and normal
capacity limits. Machine receipts use the `rss` origin, with `google-news`, query and discovery
URL in private provenance. Discovery does not post candidates to Nostr.

Console JSON logs at `info` include enablement, search start/completion, queued publisher URLs,
scan totals (`found`, `queued`, `known`, `unresolved`), duration and next-scan delay. Warnings show
fetch/decoding failures and queue errors. `LOG_LEVEL=debug` additionally shows already-seen and
already-covered candidates. No research-provider credentials or article bodies are logged.

Bing News RSS discovery is also enabled by default and runs independently of Google and the
other collectors, one second after startup and 15 minutes after each completed scan. No API key
is needed; this uses the public RSS search, not Microsoft's retired Bing Search API. Configure:

```dotenv
HARVEST_BING_NEWS_ENABLED=true
HARVEST_BING_NEWS_INTERVAL=15m
HARVEST_BING_NEWS_MAX_ARTICLES=10
HARVEST_BING_NEWS_QUERIES_PER_SCAN=4
# Optional pipe-separated queries; blank uses the built-in set. Default market: en-US.
HARVEST_BING_NEWS_QUERIES=[en-US] cannabis seized|[zh-CN] 大麻 查获
```

The twelve built-in queries cover US, UK and Indian English, Simplified and Traditional Chinese,
Russian and Arabic, plus English searches naming China, Iran and North Korea.
Markets guide search results, not the event's location; coverage is best effort. Four queries
rotate per scan, so every default query runs roughly every 45 minutes. Custom queries replace
the defaults. Short keyword queries work better than compound Boolean expressions in live testing.
Bounds match Google: 1–50 queries per scan, at most 50 configured queries of at most
1000 characters, intervals from 1 minute to 24 hours, and 1–100 new URLs per scan.

Bing tracking links are unwrapped from their publisher URL parameter before intake, without
fetching the tracking redirect. Unrecognized Bing wrappers and unsafe destinations are skipped;
raw aggregator pages are never queued. Normalized publisher URLs are checked against existing
cases and captured sources across providers; durable queue admission also deduplicates concurrent
submissions. Candidates are admitted round-robin across queries, with `rss` origin and `bing-news`,
market, query and discovery URL in private research-job provenance. No candidate is posted to Nostr.

Only results with a valid RSS publication date within the last seven days (and no more than a day
in the future) qualify, since Bing may return much older matches. This date is a discovery hint,
not evidence of the event date. AI research still verifies cannabis enforcement scope, including
excluding synthetic-only stories. Bing's public RSS interface is undocumented: challenges, malformed
feeds and individual market failures are logged and retried next scan without stopping other
queries. Fetches use the pinned public-destination transport, a 2 MiB response limit, one-second
pacing and a five-minute scan deadline. Info logs show search/queue activity and scan totals
(`found`, `queued`, `known`, `unresolved`, `skipped`). Restart the backend to apply changes.
For an opt-in live smoke test against a temporary database, with no AI or publication, run
`cd backend && BING_NEWS_LIVE=1 go test ./internal/harvest -run TestBingNewsLive -v`.

Optional RSS/Atom feeds and GDELT queries feed the same durable queue without posting every
candidate to Nostr. Provider/article keys deduplicate discovery. GDELT advances bounded one-hour
windows, pauses saturated 250-record partitions instead of silently skipping, and keeps cursors.
Public activity excludes machine discovery; the all-submissions register includes every origin.

## Additional sources and updates to other reports

When research finds that the submitted page is another account of an event a report already records,
it answers `additional_source` with that report's slug instead of rejecting it. The service then
adds the new sources to the existing report: the same quotation checks apply, each corroborated
sentence gains the new source in its citation, and genuinely new details become cited sentences.
The rest of the report (title, prose, figures, status) stays as written. If the
report already cites the sources, the submission ends as `duplicate`.

Both `additional_source` and `existing_case` use the same sentence merge. Research receives the
original Markdown files under `existing-reports/<slug>.md`, alongside the compact coverage index.
A corroborating fact either copies the existing sentence into `text`, or names it in
`existing_text`; line wrapping is ignored, and the original wording stays. A genuinely new fact
must set `new_detail: true`. An unmatched, unmarked paraphrase is rejected rather than appended
as a second account. Multiple sources supporting the same new sentence share its citation.
No automatic `Update` section is created. Repeating an unchanged merge produces no new commit.
Later case steps retain the slug, earlier citations, assets and original creator, update the
event date/type and figures where supplied, and move the file when the event month changes.
Another account of the same date/type keeps the original headline and summary. Front matter
stays block YAML; legacy JSON front matter is converted back to readable YAML when merged.

Any drafting answer can also carry `report_updates` (at most five) for other existing reports: the
same source additions with optional open questions and a corrected title or summary (a correction
sets the report's status to `reported`). The sources must be ones the answer itself cites. Updates
whose quotations fail are dropped; an update that changes nothing is skipped. Sources already cited
by a report being updated do not count as overlap with another case. All changed files pass the
report check and are committed together, each refused if the report changed since research began.
The request page lists the existing reports a run changes (or would have changed).

Before the report check, photos are compared by a perceptual fingerprint (a 64-bit difference hash):
a photo that repeats an earlier one in the answer, or one the updated report already shows, is left
out, so an agency handout republished by several outlets appears once. Photos that cannot be
downloaded are kept.

A draft that fails the report check shows, per file, which fields or citations are wrong on the
request page (also kept in the workspace's `validation.log`).

## Multilingual vocabulary

The source check uses a versioned snapshot of CFI's `/api/glossary.json`, combining its glossary,
language-atlas spellings and sourced country word lists. Sync explicitly with
`node scripts/sync-glossary.mjs` (sibling CFI checkout), or pass a CFI site base URL as the argument.
The JSON snapshot is embedded in the Go binary, so research does not depend on a live CFI API.
Matching folds case/diacritics, uses Unicode word boundaries and inflection prefixes (including
`konopy`, `konopí`, `marihuanu`), and supports scripts without spaces. Common ambiguous English
words and unrelated legal jargon do not establish a keyword match.

Missing keywords no longer reject an otherwise readable source: AI checks its relevance from
article text. Migration 013 requeues old `not_relevant` outcomes caused solely by the keyword
gate, with no recorded AI research; genuine AI relevance decisions remain unchanged.

## Access challenges and archived sources

Before reserving a model run, the service rejects Cloudflare `cf-mitigated: challenge` responses,
recognizable challenge screens (including “Just a moment…”), and unreadable pages as article
text. This is separate from the multilingual vocabulary hint. It never sends a challenge page
to OpenCode as the submitted article.

When live retrieval fails or a challenge blocks the article, it checks the Internet Archive's
Wayback availability API for a successful capture of that exact original URL. Snapshot host,
original URL and timestamp are validated; replay retrieval uses the same public-destination
checks as live sources. No archive lookup is attempted for unsafe destinations. A readable,
unchallenged capture is required before AI starts. No model call or reservation occurs if both
live and archived access fail. Archive lookup has a 15-second timeout; normal capture limits apply.

The request detail shows the archive URL and capture date. The model receives these explicitly,
with the original source URL; the crawl date must not be used as the article's publication date.
Independently opened archive links are saved as `sources[].archived` on new reports and updates.
Screenshots/evidence remain private as usual. Existing jobs need the backend restarted to use
these access checks; already consumed tokens and past activity are retained.

Quotation verification reuses the submitted source capture and completed `webfetch` article bodies
from OpenCode's private execution log. It checks quotations against those saved bodies, not against
the model's final answer or search snippets. Only normalized matching URLs are reused; failed,
truncated, unsupported and challenge-page tool responses are excluded. Uncaptured sources use the
same browser/HTTP retrieval and Wayback fallback described above. Captures are bounded, private,
and saved with the research checkpoint so a retry can verify them without another download or model
run. Older checkpoints can recover tool captures while their original workspace still exists.

Scope decisions such as `not_relevant` and `routed_to_cfi` do not draft report facts and do not run
the report quotation check. They retain the model's validated classification and explanation;
an empty fact list must not be reported as a source download failure.

## AI usage statistics

`/stats/` loads research usage from `/v1/stats/ai` and refreshes over the submission
WebSocket with a 30-second fallback. All time, rolling 30 days and Today (UTC) use
research start times, count retries once per run, and exclude unstarted reservations
and startup probes. SQL aggregates research runs directly, without joining receipts.
The endpoint returns only aggregate telemetry, grouped by model, peak/off-peak tier
and reported-cost availability. It never exports prompts, sessions or source evidence.

Input totals include cache reads/writes; output totals include reasoning. The frontend
uses the same cost helper as submission details: reported OpenCode costs take precedence,
otherwise known model pricing is estimated using the start-time tier. Unknown prices
are explicitly excluded from cost totals. Plan share uses the current subscription
configuration and is an allocation estimate, not additional billing. Interrupted/running
usage may be incomplete. Backend restart is required to expose the new endpoint.

## Job workspace cleanup

`WORKSPACE_PATH` (normally `backend/data/jobs`) contains disposable case checkouts alongside
private source captures. Cleanup **never descends into `sources/`** and does not remove source
captures, receipts, public AI activity, usage totals or research checkpoints in SQLite.

After each job finishes, the worker removes its unused repository copies, dependencies,
research inputs and build caches. Research retries rebuild from the saved checkpoint in a fresh
checkout. A checkout referenced by a queued publication is preserved until publication finishes;
all workspaces of a case with a running job are left alone, even if its lease has expired.
Partially created snapshots are removed when checkout preparation fails.

A janitor also runs at startup and every 30 minutes. It skips directories touched in the last
ten minutes, removes orphaned case workspaces, and prunes unused checkouts of unfinished cases.
Raw research logs, validation logs and job-local evidence are retained for **14 days** after a
case finishes (and at least that long after its directory was last touched), then the finished
workspace is removed. Unfinished cases retain their logs. Set `JOB_LOG_RETENTION=336h` to change
the retention period; accepted values range from `1h` to `8760h`. Restart the backend after changing
this setting or updating the worker. The public activity history stays available after raw logs expire.

To run the same sweep immediately, including while the worker is running:

```sh
cd backend
# Uses DATABASE_PATH and WORKSPACE_PATH, or their standard data/ defaults.
go run ./cmd/tracker -prune-jobs
```

This command starts no server, research or publication jobs and does not alter job leases.
It observes the same active-job, pending-publication and recent-directory protections.

Cannabis-related death sentences (`sentence`) and executions (`execution`) are tracker events,
subject to the other routing rules in `docs/CFI.md`. Migration 017 queues fresh research for old
CFI referrals whose explanation mentions death sentences or executions, preserving history and
usage while discarding the obsolete decision checkpoint. Other referrals and running jobs stay
untouched. Restart the backend to apply the migration and updated research validation.
