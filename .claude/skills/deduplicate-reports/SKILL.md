---
name: deduplicate-reports
description: Audit Cannabis Enforcement Tracker reports for repeated events or split case histories, verify matches, and consolidate confirmed duplicates while preserving evidence, photos and permanent links. Use for a full tracker audit or a requested country, period or category; not for automatically merging similar headlines.
---

# Audit and consolidate duplicate reports

Work from the repository root. Read `docs/WRITING.md` and follow `AGENTS.md`; the rules there govern report contents, provenance and validation.

## Scope and candidate scan

Check `git status --short` before editing. Default to the entire collection, across every year and type, when the user asks to check all reports. Describe a narrower audit accurately; never infer a tracker-wide result from one category or the first visible page.

```bash
node scripts/check-duplicate-reports.mjs --json > .cache/duplicate-report-audit.json
```

Create `.cache/` first if needed. The read-only scanner compares source URL variants, titles, places, figures, and shared names/body passages. It also compares widely separated dates so an arrest and a later judgment can be found. `--country=us` includes state reports; explicit report paths can scope a subsequent rescan. These are candidates, not verdicts; do not auto-delete or hide records based on scores. Review low-ranked shared-source and case-history matches too, since differently worded headlines can describe the same case.

Keep a review ledger of confirmed matches, distinct events and unresolved pairs, with the reason for each difficult decision. Scan reports added during a long audit before declaring its scope complete; other writers and the backend may be active.

## Verify an event match

Read both full reports and open their actual sources. Match the people, location, operation/case number, chronology and quantities. A shared article may be a multi-event roundup, a later story may mention an older case only as context, and nearby raids may repeat the same quantities. These do not establish duplication. Different unnamed suspects at the same airport or in the same city need positive evidence before merging.

One case progressing from arrest through appeal or release normally belongs in one report, dated at its latest supported step. A campaign total and an individual component may both be valid; do not erase the component merely because the larger total overlaps. If only one component of a combined report is duplicated, remove that component from the combined report and keep each independent event in its own record.

Uncertain matches stay separate with a documented reason for follow-up. Preserve source discrepancies in `open_questions`; unsupported claims must not acquire certainty through consolidation.

## Consolidate confirmed duplicates

- Choose an established canonical slug, normally the oldest record; prefer an already-established target when aliases exist. Preserve its original `code`, `added_at`, `added_at_source` and `created_by_model`. Do not invent creator or update-model metadata.
- Keep supported unique facts, all relevant source evidence and useful open questions. Combine repeated prose, not quantities: never add two accounts' figures for the same seizure. Prefer an authoritative final figure; retain differing counts, dates or values transparently.
- Remap every citation to the merged source list. A URL variant may be the same article; preserve useful archive metadata. Add citations only to statements those sources support, not to an arbitrary first sentence to satisfy validation. Each factual sentence needs a citation and every retained source must be cited.
- For a later step, update the date/type and move the canonical file into its new year/month folder without changing its slug, code or addition metadata. Keep `disputed`; checked additions do not verify older unchecked prose. Use `reported` when retained facts could not be checked, explaining actual gaps.
- Preserve every distinct photo, video and licensed cover. Compare before/after inventories. Identical URLs may be collapsed; different URLs may depict the same handout, but different photographs on one source page must not collapse together. Use `scripts/duplicate-media.mjs` as a review aid and visually inspect uncertain matches. Retain a copy/reference for each unique image, and never guess direct media URLs. Do not delete cover assets or private evidence archives as part of removing duplicate Markdown.
- Remove the duplicate report files only after their evidence is retained. Add every retired slug and code to `src/lib/report-aliases.js`, pointing directly to the surviving slug. Retarget existing aliases if their target is itself consolidated; avoid chains and cycles. The existing Astro redirects and saved-submission links consume this map.

## Validate and report

Run `npm run codes -- <changed report paths>` without replacing existing identities. Archive changed reports' pages with `scripts/archive-sources.mjs`. Report unavailable evidence honestly; archive lookup failure is not successful verification. Run media/source mutation scripts sequentially for the same file so one cannot overwrite another's edits.

Run the report/alias tests, `npm run build` (including its tests), and `node scripts/load-test.mjs`. Read the generated report HTML and verify retired report URLs and `/r/<code>/` pages redirect to live targets. Check the JSON index excludes retired entries and lists every surviving report once. Verify filter/search results in the browser for representative merged cases. For a running local dev server, POST `/__dev/refresh-reports` after edits, then verify its live index; the report watcher intentionally does not refresh automatically.

Rescan after consolidation and review remaining plausible pairs. State the number of reports screened, confirmed duplicates consolidated, unresolved cases and validation limitations. “No further confirmed duplicates found” is defensible; “guaranteed duplicate-free” is not. List the updated cases and preserve the review ledger for further work. Commit only this task's files when requested; creating this skill or auditing data does not authorize a push or a backend publishing change.
