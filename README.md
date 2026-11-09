# Cannabis Enforcement Tracker

An open-source, global record of cannabis **eradication, seizures, raids, arrests, prosecutions,
sentences, executions and deaths**. It documents what the prohibition of cannabis does in practice:
the human, economic and cultural cost that the text of a law does not show.

Every report is short, plain and sourced: each sentence cites a source that was opened and read,
Wikipedia-style (`[@1]`). The build refuses a report without sources.

It is a companion to the [Cannabis Freedom Index](https://cannabisfreedom.fyi/) (CFI), which scores
what each country's laws allow. Country ids are the CFI's, so every report links to the law behind
it, and the CFI can show the tracker's reports on its country profiles through the JSON API below.

Site: <https://tracker.cannabisfreedom.fyi> (static, built with [Astro](https://astro.build/)).

## Development

`npm run dev` hot reloads frontend code. Report files, their photo manifest
(`src/data/media.json`) and Git commits do not trigger reloads, so the backend can keep
publishing while you work. Use **Refresh reports**
in the header to resync reports, indexes, report routes and Git history, then reload
the current page (preserving its URL and filters). Restarting the dev server also
loads current reports. The button and refresh endpoint exist only in development;
production builds always load the latest reports.

## Pages

| Address | What it shows |
|---|---|
| `/` | **Map**: a full-screen map of all reports (the view is kept in the address, `#map=zoom/lat/lon`, so it can be shared) (a diamond marker per type; grouping crowded markers into clusters is an option in the Layers panel) with a pane on the right: the newest reports; filter by type, country and free text, use synchronized time controls on the map and in the panel, or select a period in the timeline; filters are kept in the address (`/?type=sentence&country=ge&year=2025`) |
| `/reports/` | **Reports**: the same filterable list as a page of its own, with an RSS subscription button and automatic loading as you scroll |
| `/<year>/`, `/<year>/<month>/` | The archive by date: a year's months, and every report of a month (`/2026/08/`) |
| `/report/<slug>/` | One report: figures and structured details with their sources, the law at the time (CFI score that year, CFI history before and since), text with citations, sources, map pin, link to the CFI profile, related reports; semantic article markup and Article metadata support native browser reader views; availability is decided by the browser |
| `/country/<id>/` | A country's or state's reports, totals and map (`/country/ng/`, `/country/us/ca/`) |
| `/country/` | All 220 CFI countries and territories, including places with no reports; states and provinces listed separately |
| `/stats/` | Totals, reports per month since the first report, by type, region and year, top countries; what the reports record (agencies, products, settings, case stages, arrest-to-sentence counts, smuggling routes) |
| `/agency/`, `/agency/<id>/` | The agencies of the registry (`src/data/agencies.json`) with how many reports name each; one agency's reports and totals |
| `/changelog/` | Paginated commit history: reports added, edited, deleted and renamed, report descriptions, countries, categories and changed fields, plus a timeline of report changes; refreshed at build time |
| `/feed.xml` | RSS feed of the 50 newest reports |
| `/sitemap.xml`, `/robots.txt` | Written after the build from the pages it produced (`scripts/lib/sitemap.mjs`): every page except redirects and pages marked `noindex` |
| `/about/` | What a report is, the types and statuses |
| `/api/…` | JSON, see below |

The map is [MapLibre GL JS](https://maplibre.org/) on [OpenFreeMap](https://openfreemap.org/)'s
dark and light (positron) vector styles (OpenStreetMap data, no API key), switched with the site's
dark/light theme (the button in the top bar; the reader's choice is remembered, otherwise the
system's is followed). The style URLs are `MAP_STYLES` in `src/lib/map.js`. Each type has a diamond marker with a pictogram (`typeIcon` in `src/lib/types.js`,
also used by the legend), in warm colours for what is done to land and people and cooler ones for
the courts; there is no green, which belongs to the Landrace layers.

On the Reports page, scrolling near the bottom automatically appends the next 30 reports.
Filters reset the loaded list; year/month navigation remains available. Browsers without
IntersectionObserver keep the **Show more** button. After building the site and installing the
browser, run `node scripts/check-report-scroll.mjs` for the scrolling regression checks.

The map's quick period buttons and the panel's **Time** selector share one time filter.
All report listings show the latest incidents first. Within each incident day, the most recently
added reports come first. Time filters, calendar days, chart bars and map markers use incident
dates. A historical case added today stays under its historical incident date.

Report detail pages show relative times such as **Added 3 hours ago · Updated 1 minute ago**,
with exact UTC timestamps on hover. Updated appears only after a later committed content edit.
Git addition times have been written into existing reports and are treated as relevant tracker
times. Update times are derived from Git history and are not stored in report files. Report
lists show the incident date without addition/update lines.

Choose Last 7 days, Last 30 days, Last year or All time for a preset, or **Calendar dates…**
for a year, month or day. Calendar selections switch off rolling-window restrictions; choosing
a preset clears calendar selections. The map highlights **Calendar** while calendar dates are
active. The Reports page offers only All time and Calendar dates, without relative presets.
Shared links retain the same selection, and older links with both recent and calendar
parameters resolve to the calendar selection.

The Reports timeline includes a count scale, gridlines and zero-day baseline markers.
Use **Clear all filters** to reset search, category, country and time selections to All time.
Timeline bars show category-count tooltips on hover or keyboard focus. Selecting a year or month
drills into that period; selecting a day filters both the report list and map to that exact date.
Day filters are shareable as `?date=2026-10-26` and combine with the other filters. Use **Clear day**
or select the same day again to remove the day filter.
Browser tooltip regressions (rapid hover, keyboard access and scrolling) can be checked with
`node scripts/check-timeline-tooltip.mjs` after `npm run browser-install`.

### Landrace layers

The map's "Layers" switch shows [Landrace.wiki](https://landrace.wiki/)'s growing areas (their
outlines; on by default), growing regions (dashed outlines) and accessions: where landrace cannabis has been grown and collected for generations, next
to where it is destroyed. The data is a snapshot in `public/layers/`: `landrace.geojson` (points),
`landrace-areas.geojson` and `landrace-regions.geojson` (outlines from the wiki's `Module:Boundaries`
data pages, simplified in proportion to each shape), © Landrace.wiki contributors, **CC BY-SA 4.0**;
point coordinates are already offset by about 500 m by Landrace.wiki. Each file loads only when a
layer that needs it is on. Refresh it by hand with

```bash
node scripts/sync-landrace.mjs
```

(about 17 requests to their API, three seconds apart). Their API page asks to be emailed (privacy@landrace.wiki) before
any regular harvest, so don't put it on a schedule without that.

## Adding a report

1. Find the event in a reliable source and **open it**. Prefer the agency's own statement or court
   record, plus independent reporting. Use only facts the sources state.
2. Create `reports/<year>/<month>/<slug>.md` for the date of the event. The file name is the slug and
   the report's address: lower case, digits and hyphens, unique across all years
   (`ndlea-edo-igbanke-farms`).
3. Fill in the front matter and write the text (format below). Cite every sentence with `[@n]`, the
   number of the source in `sources`. Every source must be cited at least once.
4. Run `npm test` (a second) and `npm run build`.

The full guide is [docs/WRITING.md](docs/WRITING.md). For agents: to add a report from a URL, see
[.claude/skills/add-report/SKILL.md](.claude/skills/add-report/SKILL.md); to find new events,
[.claude/skills/find-reports/SKILL.md](.claude/skills/find-reports/SKILL.md).

Writing: lead with what happened to people (who was arrested, jailed, lost their crop or land),
then the figures, then context. Use the source's numbers, and say whose numbers they are ("the
agency valued them at …"). No law-number lists, no adjectives the source does not support.

## Data format

```yaml
---
code: "7KQ2MX"                  # REQUIRED permanent CET id; npm run codes -- <report paths> assigns it
added_at: "2026-08-17T14:23:05Z" # assigned automatically; preserve on edits and moves
date: 2026-08-17                 # when it happened (a campaign: when it ended or was announced)
country: ng                      # a CFI country file: "de", "th"; a state: "us/ca", "au/act"
place: Igbanke forest, Orhionmwon, Edo State     # optional
coordinates: [6.38, 6.17]        # optional, [lat, lon]; without them the dot goes to the country's middle
precision: district              # needed with coordinates: exact | locality | district | region | country
type: eradication                # eradication | seizure | raid | arrest | prosecution | sentence | execution | death
also: [arrest]                   # optional: other types the event involved
title: NDLEA destroys two cannabis farms in Edo and makes eleven arrests   # ≤ 120 characters
summary: >-                      # ≤ 400 characters, used in lists, cards and the JSON
  Nigeria's drug agency destroyed 1.85 hectares of cannabis …
figures:                         # optional; only numbers a source states
  arrested: 11                   # people detained
  charged: 1                     # people charged or tried
  kg_seized: 12.4                # cannabis taken or destroyed, kg
  plants_destroyed: 48000
  hectares: 1.85                 # fields destroyed
  prison: 5 months 24 days (time served)   # a sentence, as stated
  fine: { amount: 500000, currency: GEL }
  value: { amount: 9600000, currency: PHP }   # the authorities' estimate
  deaths: 1
  convicted: 1                   # also: acquitted, sentenced, released, injured, plants_seized,
                                 # kg_destroyed, sites, seeds, vehicles, cash (see docs/WRITING.md §5)
details:                         # optional; ids from src/data/vocabularies.json and agencies.json
  agencies: [ng-ndlea]           # registry id, or the agency's name as published
  agency_types: [anti-narcotics]
  products: [{product: flower, kg: 12.4}]
  activity: [cultivation]
  setting: [field]
  environment: [outdoor]
  charges: [cultivating cannabis]
  case_stage: charged            # investigation | charged | trial | convicted | sentenced | acquitted | appeal | final | executed
  sentence_types: [prison]
  force: [none-reported]
  period: {start: 2026-04-02, end: 2026-04-04}
  date_precision: exact          # exact | approximate | publication
  region: Edo State
  route: {from: [ma], to: [es], crossing: Algeciras}
  part_of: ndlea-edo-campaign-2026   # a campaign report this event is part of
  related: [edo-farm-owner-sentenced]
data_sources:                    # which sources (by number) state each figure and detail
  figures.arrested: [1]
  details.agencies: [1, 2]
cover:                           # optional; a local picture with an open licence
  image: ../../../src/assets/reports/example.jpg
  alt: What the picture shows
  credit: Photographer / agency
  licence: CC BY-SA 4.0
  url: https://commons.wikimedia.org/wiki/File:…
  media_url: https://upload.wikimedia.org/… # direct original image; required for new collected covers
media:                           # optional; photos or video of the event that can't be copied here
  - kind: photo                  # photo | video
    url: https://…               # the page this photo is on
    media_url: https://…         # photo/video URL; required for every newly collected photo or video
    publisher: La Nación (Paraguay)
    caption: Agents pull up plants in a cleared forest plot
created_by_model: unknown         # REQUIRED: provider/model#effort when known; unknown only if creator is unavailable
submission: 0123456789abcdef0123456789abcdef  # optional: the submission the service created it from
status: verified                 # verified (checked against opened sources) | reported | disputed
tags: [ndlea, farmers]           # optional, lower-case-with-hyphens
sources:                         # at least one
  - title: NDLEA Destroys Cannabis Farms Worth 4628kg, Arrests 10 Suspected Traffickers In Edo
    url: https://saharareporters.com/2026/08/17/…
    publisher: Sahara Reporters
    date: 2026-08-17             # optional: when the source was published
    archived: https://web.archive.org/web/…   # optional
    language: es                 # optional: two-letter code if not English
---

The Edo State command of Nigeria's National Drug Law Enforcement Agency (NDLEA) destroyed two
cannabis farms … [@1] A day later, … [@2]
```

Run `npm run codes -- <new/updated report paths>` before building or committing skill-written
reports. Codes are six-character strings (quote all-digit values), assigned once and preserved
on updates, slug changes and date-folder moves. The backend assigns them automatically.
`npm run codes` without paths fills missing codes across the repository.

The rules live in two places, both run by `npm run build`:

- `src/content.config.ts`, the zod schema: fields, types, the CFI country ids, at least one source,
  coordinates need a precision.
- `scripts/reports.test.mjs` (`npm test`): the file sits in the folder of its date, slugs are
  unique, every citation names a source and every source is cited.

Citations are turned into numbered links by a Markdown plugin in `astro.config.mjs` (the same one as
the CFI's); it also stops the build on a citation to a missing source.

### Country ids and the CFI widget

`src/data/countries.json` is a copy of the CFI's places: ids, names, regions, legal status, and what
the **Cannabis Freedom Index widget** on each country page shows (score, tier, rank, the six segment
scores, summary, review date, score history). Names and segment scores come from a CFI checkout; the
score and tier as the CFI publishes them (including national estimates and states) and the history
come from the live site. Refresh it with:

```bash
npm run countries:sync -- ../cannabis-freedom-index
```

(`--local` reads the checkout's built `dist/` instead of the live site.) In the browser the widget
then refreshes itself from the CFI's own `https://cannabisfreedom.fyi/api/<id>.json` (current score,
summary, score line since 1900, the latest law changes that moved the score, cannabis use). States
have no such file and keep the built-in copy. Report pages show a compact version under "The law
there".

## Build

```bash
npm install
```

```bash
npm test
```

```bash
npm run build
```

`npm run build` runs the tests first and writes the site to `dist/`. Set `SITE` and `BASE_PATH` to
build for another address.

Search and sharing: every page has a canonical URL, a description, Open Graph and Twitter card
tags and a robots directive (`src/layouts/Base.astro`). Report pages add `Article` and
`BreadcrumbList` structured data with the place, sources and added/updated times; the home page
adds `WebSite`, `Organization` and `Dataset`. Working views (submissions, drafts, 404), countries
and agencies without reports and changelog pages after the first are `noindex, follow` and stay
out of the sitemap. Pages without a cover picture share `public/og.png`, drawn by
`node scripts/og-card.mjs`.

The changelog requires a Git checkout with full history (`git fetch --unshallow` for a shallow clone); partial history is labeled on the page. In development, the changelog checks Git HEAD once per second, clears cached history and refreshes an open changelog after a commit or branch switch. Restart an already-running dev server once after installing this change. `npm run dev` starts a local dev server.

## JSON API

All files are static and rebuilt with the site. Dates are `YYYY-MM-DD`, coordinates `[lat, lon]`.

### Files

| File | What |
|---|---|
| `/api/reports.json` | every report |
| `/api/reports/<year>.json` | one year's reports (`/api/reports/2026.json`) |
| `/api/latest.json` | the 100 newest, with `total` |
| `/api/country/<id>.json` | one country or state |

`/api/index.json` is the site's own compact index for the map and the filters; it may change
without notice.

### `/api/reports.json`

Every report, newest first.

```json
{
  "version": 1,
  "generated": "2026-10-29T00:00:00.000Z",
  "count": 8,
  "types": { "eradication": { "label": "Eradication", "color": "#5f9e57" }, "…": {} },
  "by_type": { "eradication": 5, "seizure": 1, "…": 0 },
  "reports": [ /* report objects, below */ ]
}
```

### `/api/country/<id>.json`

One file for **every** CFI place, with or without reports, so a CFI profile can always fetch its
own: `/api/country/ng.json`, `/api/country/us/ca.json`. A country's file includes its states'
reports (`us.json` lists `us/ca`'s).

```json
{
  "version": 1,
  "id": "ng",
  "name": "Nigeria",
  "url": "https://tracker.cannabisfreedom.fyi/country/ng/",   // null if no reports
  "count": 1,
  "totals": { "reports": 1, "arrested": 11, "plants_destroyed": 0, "hectares": 1.85, "kg_seized": 0, "deaths": 0 },
  "by_type": { "eradication": 1, "…": 0 },
  "latest": "2026-08-17",                                       // null if no reports
  "reports": [ /* report objects */ ]
}
```

### Report object

```json
{
  "id": "ndlea-edo-igbanke-farms",
  "url": "https://tracker.cannabisfreedom.fyi/report/ndlea-edo-igbanke-farms/",
  "date": "2026-08-17",
  "added_at": "2026-08-17T14:23:05Z",
  "added_at_source": "recorded",
  "updated_at": null,
  "country": "ng",
  "country_name": "Nigeria",
  "place": "Igbanke forest, Orhionmwon, Edo State",
  "coordinates": [6.38, 6.17],
  "precision": "district",
  "type": "eradication",
  "also": ["arrest"],
  "title": "NDLEA destroys two cannabis farms in Edo and makes eleven arrests",
  "summary": "…",
  "status": "verified",
  "figures": { "hectares": 1.85, "arrested": 11 },
  "tags": ["ndlea", "farmers", "legalisation-debate"],
  "sources": [{ "title": "…", "url": "…", "publisher": "Sahara Reporters", "date": "2026-08-17", "archived": null }]
}
```

`coordinates` falls back to the middle of the country (`precision: "country"`) when the report gives
none. `figures` holds only the keys a report states. Fields may be added in later versions; existing
fields keep their meaning while `version` is 1.

## Coordinate audit

`npm run coordinates:check` flags explicit coordinates outside their mapped country, identifies
other mapped territories, and suggests sign/order mistakes to verify. It also documents reviewed
offshore and locality references. `npm run coordinates:duplicates` clusters identical or
near-identical pins reused across reports so unrelated localities can be reviewed; it does not
move pins. Both are offline review aids, with no automatic report changes or rejection. See
[coordinate checks](docs/COORDINATES.md) for JSON output, individual-report checks, offshore
cases and dataset limitations.

## Photo thumbnails

`node scripts/media-manifest.mjs [--retry]` describes every photo a report links with a
`media_url` in `src/data/media.json` (committed): the SHA-256 of the original, its width, height and
type, a ThumbHash placeholder and whether it could be fetched (`ok` or `unavailable`, with the
error). Only a 320px AVIF thumbnail is kept, in `public/media/<ab>/<cd>/<sha256>.avif`, committed and
served at `/media/`. Pages reserve each photo's size and show its blurred placeholder until it
loads. Report lists and photo grids of three or more use the thumbnails; a grid of one or two
photos, and enlarging a photo, load the original from its publisher through the image proxy.
`MEDIA_BASE` at build time serves the thumbnails from elsewhere. Videos that play in place (a video
file, or YouTube) are shown as a poster with a "Play video from <site>" button and contact nobody
until it is clicked; the script makes their 640px poster from YouTube's preview image or a frame of
the file (ffmpeg). The service does the same for the reports it commits. The submission service does this
for every report it commits: it fetches new photos through its public-address-only client, runs the
script with `--input`, and commits `src/data/media.json` and the new thumbnails separately
("Add photo thumbnails: …"), before pushing.

## Archived copies of sources

`node scripts/archive-sources.mjs` looks up each source and media page in the Wayback Machine and
writes the newest snapshot into the report as `archived:`. With `--save` it also asks the Wayback
Machine to save pages it has no copy of yet; with slugs it does only those reports; `--dry` writes
nothing.

Report `added_at` is a UTC ISO timestamp with seconds. `added_at_source` is `recorded` or `git`,
identifying its origin. New manual reports receive timestamps through `npm run codes -- <paths>`;
automated reports receive them from the service. Existing Git addition dates are persisted with
`npm run times`, following recognized renames and preserving the original time across edits.
The API also exposes `updated_at`, computed from committed content changes in Git, with
timestamp-only migrations ignored. Do not store `updated_at` in report files. Unknown API times
are null. Full Git history is needed to recover old times; shallow checkouts leave unknown
addition times unset. Build time, file modification time and receipt time are never substitutes.

## Built for thousands of reports

The tracker is organised by date: files in `reports/<year>/<month>/`, archive pages per year and
month, a timeline per month. Pages do not grow with the number of reports: the home page carries the
30 newest and loads one compact, cached index (`/api/index.json`, about 135 KB gzipped for 5,000
reports) for the map, the filters and "Show more"; the map clusters dots; report pages load the same
index for their map instead of embedding other reports; a country page shows its 20 newest in full
and the rest as one line each, by year.

```bash
node scripts/load-test.mjs 5000
```

builds the site from 5,000 synthetic reports in `.cache/load-test/` (not in `reports/`) and prints
the build time and the size of the main pages and files. With 5,000 reports: about 18 s, home page
37 KB, a report page 8 KB.

## Deployment

`.github/workflows/deploy.yml` checks `main` every 30 minutes, at minutes 7 and 37 of each hour (UTC).
It runs the tests, builds and publishes to GitHub Pages only when the commit differs from the last
successful deployment workflow run. Unchanged commits skip the tests, build and deployment; failed
builds or deployments are retried at the next scheduled check. **Run workflow** on `main` forces an
immediate test, build and deployment, even for an unchanged commit. A test failure stops deployment.
GitHub schedules can be delayed during busy periods. To set it up once:

1. Push the repository to GitHub.
2. Settings → Pages → Source: **GitHub Actions**.
3. Settings → Pages → Custom domain: `tracker.cannabisfreedom.fyi` (also in `public/CNAME`), and at
   the DNS host a `CNAME` record `tracker` → `<user>.github.io`. Turn on "Enforce HTTPS" once the
   certificate is issued.

The site address and base path come from the Pages settings, so the build also works on
`<user>.github.io/<repo>/` before the domain is set.

## Planned

- AI-assisted collection: drafts from news and agency feeds, saved with `status: reported`, then
  checked by a person against the opened sources before they become `verified`.
- Archived copies of every source.
- Enforcement news on the CFI's country profiles, from `/api/country/<id>.json`.

## Licence

Code: MIT. Report texts and data: CC BY 4.0. Pictures keep their own licences, stated with each.

## Public submissions and Nostr service

The optional Go service in `backend/` adds anonymous source intake, durable receipts, Nostr mentions
and replies, sandboxed OpenCode research, live model/session activity, and automatic local report commits after validation. Astro and the public
report JSON API remain static. Pages: `/submission/?id=…` for request details, and `/submissions/` for the source form and
live request table. `/submit/` redirects to `/submissions/`. AI-created reports record `created_by_model` as `provider/model#effort` when available, shown on the report detail page. Reports the submission service creates also record `submission`, the id of the submission they came from; the report page links it and, when the submissions API is configured, adds when it was submitted, how long the research took and its AI cost. Agents writing reports by hand leave it out, and updates keep the original. For example, `alibaba-token-plan/qwen3.8-flash#xhigh` records `xhigh` as the reasoning effort setting. Include the actual effort whenever known; omit only the suffix if effort is unavailable, and never guess it.
Updates preserve the original creator. Open questions are saved in report front matter
and shown beneath its sources. Local failed submissions have a restart control; no GitHub App is needed.

Research can also run on separate machines: with `RESEARCH_MODE=remote` the backend keeps the
database, intake, harvesting and publishing, and research workers (`cmd/tracker-worker`, for example
at home) take the jobs, open the pages and run the AI. See [docs/WORKERS.md](docs/WORKERS.md).

See [docs/NOSTR-SUBMISSIONS.md](docs/NOSTR-SUBMISSIONS.md) for setup, HTTP contracts, deployment,
privacy, backups, research isolation and rollout requirements. Use `backend/config.example.env`
for private service configuration and `.env.example` for public-only Astro build settings.
For JavaScript-rendered sources and archived screenshots/SEO metadata, install Chromium once with
`npm run browser-install` (`SOURCE_BROWSER=true` by default).
Local `backend-dev` loads the ignored root `.env`; it uses OpenCode’s saved credentials and the `RESEARCH_MODEL` set in `.env` or the process environment (`provider/model#effort` when the effort setting is known). There is no built-in model default. Do not prefix secrets with `PUBLIC_`. The form
stays unavailable in production until `PUBLIC_SUBMISSIONS_API_URL` is set. For local work, run
`npm run backend-dev` and `npm run dev` in separate terminals; the frontend connects to localhost automatically. Never include service secrets in an
Astro build. Backend checks: `cd backend && go test -race ./... && go vet ./...`.

The submission source vocabulary is synced from CFI's public `/api/glossary.json` with
`node scripts/sync-glossary.mjs [../cannabis-freedom-index|https://cannabisfreedom.fyi/]`.
The generated snapshot is embedded in the backend; rebuild/restart after syncing.

CFI glossary and language-atlas explanations also appear throughout tracker prose with dashed underlines and
hover/focus/tap tooltips. `node scripts/sync-glossary.mjs` updates both the backend recognition
vocabulary and `src/data/glossary.json`. Markdown gets term markup during builds; the shared
layout also marks summaries and live previews. Links, headings, form controls, citations, code,
maps and raw AI transcripts are left untouched. Atlas terms such as bhang also link to their
CFI article. Each glossary term is marked once per page.

## Structured details: maintenance

- `node scripts/backfill-details.mjs [--write]` fills `details` of existing reports from their tags,
  where a cited sentence of the report bears the value out (its citations become `data_sources`).
- `go run ./cmd/tracker -enrich N` (in `backend/`) asks the research agent for quoted structured
  values of up to N reports without details, checks every quotation against the opened sources,
  writes the reports (no commit) and logs conflicts with existing figures to
  `backend/data/audits/enrich-<date>.jsonl` for review.
- `node scripts/agency-names.mjs [--apply]` lists agency names reports use that are not in
  `src/data/agencies.json`; after adding an agency there, `--apply` rewrites its names to the id.
- `node scripts/sync-cfi-changes.mjs` copies the CFI's dated law changes to
  `src/data/cfi-changes.json` for the report pages' legal context (run with `sync-countries.mjs`).
