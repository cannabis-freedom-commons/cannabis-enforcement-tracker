# Notes for agents

The Cannabis Enforcement Tracker: sourced reports of cannabis eradication, seizures, raids, arrests,
prosecutions, sentences, executions and deaths, built with Astro. Companion to the Cannabis Freedom
Index (country ids are the CFI's).

- **Scope:** cannabis enforcement only. Synthetic cannabinoids and their precursors are out of
  scope, including hemp used only as a carrier for them. Mixed events qualify only with a separately
  supported cannabis enforcement component; keep synthetic quantities out of cannabis figures.
  See `docs/CFI.md` for the full rule.
- **To add a report from a URL** the user gives, follow `.claude/skills/add-report/SKILL.md`.
- **To find and add reports**, follow `.claude/skills/find-reports/SKILL.md`. Start with
  `node scripts/coverage.mjs`.
- **To enrich existing reports** (bring older reports up to the current rules: photos, archived
  copies, second sources, coordinates, structured details, open questions), follow
  `.claude/skills/enrich-report/SKILL.md`. Start with `node scripts/report-gaps.mjs`.
- **To audit or merge duplicate reports**, follow `.claude/skills/deduplicate-reports/SKILL.md`.
  Start with `node scripts/check-duplicate-reports.mjs --json`; candidates require source verification.
- **What belongs here and what goes to the CFI history** (changes in the law, famous and landmark
  cases, statistics, events before 2000): `docs/CFI.md`.
- **How a report is written** (one event, sources, figures, pictures): `docs/WRITING.md`.
  Data format and JSON API: `README.md`.
- Every report file **must** include a permanent `code`: six Crockford base32 characters,
  stored as a YAML string without the `CET-` display prefix. For skill-written reports run
  `npm run codes -- <report paths>` before building/committing; the backend assigns it itself.
  Preserve the original code on updates, renames and date-folder moves.
- New reports record an immutable UTC `added_at` timestamp, separate from the incident `date`.
  `npm run codes -- <report paths>` assigns it for manual reports; the backend assigns it itself.
  Preserve it and any `added_at_source` on updates, renames and date-folder moves. Existing Git
  addition times are persisted as relevant tracker times. Last-update times are derived from Git
  content history, ignoring timestamp-only migrations; do not store `updated_at` in report files.
- Every report file **must** include `created_by_model`: the exact model identifier used by the
  writing agent, including provider and reasoning effort when known, as `provider/model#effort`
  (for example `alibaba-token-plan/qwen3.8-flash#xhigh`, where `xhigh` is the effort setting).
  Never omit an available effort setting or guess an unavailable one. Agents writing Markdown
  set it themselves;
  the submission service injects it from the OpenCode run configuration. Never omit it or copy a
  sample/current model onto older work. Use `unknown` only when the actual creator model is not
  available (notably historical imports), and make that limitation clear. Preserve the original
  creator on updates. Do not add update-model metadata.
- Every sentence cites a source that was opened (`[@n]`); every source must be about the same event.
- **Incidental finds:** when research turns up useful information or another source for an existing
  report, read that report, confirm the case match, and update it in the same run without waiting
  for a separate request. Add corroborating citations or genuinely new details to the matching
  report, keeping different events separate. Follow `docs/WRITING.md` and validate all changed reports.
- Photos: copy one into `src/assets/reports/` only with an open licence or a no-copyright government
  work; link all others as `media`. After adding media run `node scripts/media-manifest.mjs`: it
  records each photo's size and placeholder and commits only a 320px thumbnail (`public/media/`). Every newly collected photo or video must include its direct
  `media_url` alongside the source-page `url`, one media entry per photo or video. Never guess a media
  address.
- The site must stay fast with thousands of reports: no page may embed all reports; use the
  indexes in `src/lib/reports.js` and `/api/index.json`. Check with `node scripts/load-test.mjs`.
- Check with `npm run build` (runs `npm test` first) and read the HTML in `dist/`. Don't start dev
  servers unless needed.
- For UI work, build only the pages you changed: `npm run build:pages -- / api/index.json` (add
  `--no-test` to skip the tests) writes them to `.cache/page-review/` in seconds instead of minutes.
  Before deploying, or after changing data, routing or shared links, run the full `npm run build`.
- Others may work here at the same time: check `git status` and commit only your own files.

## Submission service

`backend/` is the persistent Go intake, Nostr, OpenCode research and publishing service; Astro stays static.
Read `docs/NOSTR-SUBMISSIONS.md` before changing it. Run `cd backend && go test -race ./...`
and `go vet ./...` for service changes, plus the site's checks for UI changes.
The user has requested automatic processing and publication without human approval gates.
The publisher commits only validated report files directly to the configured local repository;
it never includes unrelated staged changes or calls GitHub to publish. `committed` is distinct from
remote deployment. Preserve minor discrepancies and missing details in `open_questions` and show
them on report pages; reject unsupported/untrustworthy events rather than every open question.
Local failed submissions can be restarted through loopback-only development controls.
OpenCode is the actual research harness: a sandboxed local CLI for macOS development, or a
read-only container for deployment. Give it only research-provider credentials, never Nostr
keys, GitHub credentials or the operational database. Automated report status is assigned by the
service: `verified` when every retained fact passes a quotation check against opened/captured
sources, otherwise `reported` for retained unchecked claims. Open questions alone do not prevent
verification. Preserve `disputed`; checked additions do not verify unchecked older prose.
Only deliberate public assistant/tool activity belongs in the live AI transcript; never expose
raw sources, private reasoning, tool arguments/output, credentials or host filesystem paths.
