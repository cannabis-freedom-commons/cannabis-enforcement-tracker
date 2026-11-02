---
name: find-reports
description: Find new cannabis enforcement events (eradication, seizures, raids, arrests, prosecutions, sentences, executions, deaths) and add them as sourced reports to the Cannabis Enforcement Tracker. Use when asked to find, collect, add or research more reports, cases or news, for a country, region, type or period (e.g. "find 5 new reports", "/find-reports Africa", "add recent raids in Thailand", "/find-reports sentences 2026").
---

# Find and add reports

Goal: a few **new, real, properly sourced** reports per run, spread over countries and types, written
by `docs/WRITING.md`. Quality over quantity: three good reports beat ten thin ones.

Arguments (all optional): a count (default 5), a country id or name, a region, a type, a period.

## 1. See what is there

```bash
node scripts/coverage.mjs
```

Prefer what is thin: regions with "none yet", types with few reports (raids, arrests,
prosecutions, sentences, deaths are rarer than eradication), countries the Cannabis Freedom Index
scores harshly. For one country: `node scripts/coverage.mjs --country <id>`.

## 2. Search

Search in the country's own language as well as English; most enforcement news is local. Run several
searches in one turn. Useful patterns:

| Type | English | Local examples |
|---|---|---|
| eradication | "cannabis plantation destroyed" "hectares" "uprooted" | es "erradicación marihuana hectáreas"; pt "erradicação maconha"; id "ladang ganja dimusnahkan"; fr "plantation de cannabis détruite"; tl/ph "marijuana uprooted" |
| seizure | "tonnes of cannabis seized" "hashish seized" | es "decomiso marihuana toneladas"; fr "saisie de résine de cannabis"; ar "ضبط حشيش" |
| raid | "cannabis farm raid" "dispensary raid" "cannabis club raided" | de "Razzia Cannabis"; th "บุกค้น กัญชา" |
| arrest | "arrested with cannabis" "arrested for marijuana" | es "detenido con marihuana" |
| prosecution / sentence | "jailed for cannabis" "sentenced cannabis" "court cannabis" | fr "condamné cannabis"; ms "hukuman ganja" |
| execution / death | "executed cannabis trafficking" "hanged cannabis" "shot during drug raid" | |

Good starting points (open them, they are sources only when they report the event):

- Agencies' own news: NDLEA (Nigeria), PDEA and PNA (Philippines), Senad (Paraguay), BNN (Indonesia),
  CNB (Singapore), ONCB (Thailand), NCB and state police (India), California DCC / governor's office,
  DEA press releases (US, public-domain photos), Europol, national police and customs.
- Court and rights reporting: Harm Reduction International, Death Penalty Information Center,
  Amnesty International, Human Rights Watch, Transformative Justice Collective (Singapore).
- Wire services and national papers.

Prefer events from the **last 12 months**; older ones only when notable (an execution, a landmark
sentence, a record campaign).

## 2b. Here or in the CFI?

Apply the substance scope in `docs/CFI.md` first: skip synthetic cannabinoids and their precursors,
including hemp used only as their carrier. A mixed event needs a separately supported cannabis
enforcement component; never count synthetic quantities as cannabis.

Only single enforcement events since 2000 that change no law are reports (`docs/CFI.md`). A change in
the law, a precedent, a famous or landmark case, national statistics or an execution found on the way
goes to the Cannabis Freedom Index history (`../cannabis-freedom-index`, `changes.yaml`, added without
asking as its `docs/EXPAND-COUNTRY.md` says).

## 3. Check before writing

For each candidate:

1. **Open the article.** Read its own publication date (not the page header's date) and confirm the
   event's date, place, agencies, figures and what happened to the people involved.
2. **Not a duplicate:** `node scripts/coverage.mjs --url <url>`, and look at the country's reports.
   A later phase or a new step in a known case is a new report only if it is a new event (a sentence
   after an arrest report: update the existing report instead, and move it to the date and folder
   of the latest step).
3. **Find a second source** for the same event if you can: the agency's statement plus a news report,
   or two outlets. Every source must be about **this** event (docs/WRITING.md §1).
4. **Look for a photo of the action** (docs/WRITING.md §7): a copy (`cover`) only with an open
   licence or a no-copyright government work; otherwise list good photos and video as `media` links.
   Look at an image before you use it: it must show this event. Add **every** photo or video of the
   event you find as `media`, so the record is complete. For every collected photo or video, record
   `media_url` as its direct absolute HTTP(S) photo/video address and keep `url` as its source page.
   Use one entry per photo or video, including multiple photos on one page. Covers also keep the original
   `media_url`. For videos, use the file/stream URL or dedicated video-host page. Extract addresses from the
   opened page, never guess them; if an address cannot
   be recovered, note the unavailable photo in `open_questions` instead of inventing an entry.
5. Drop the candidate if the facts cannot be confirmed in an opened source. Never invent or fill
   gaps.

## 4. Write

- File: `reports/<year>/<month>/<slug>.md` for the event's date; slug short and descriptive
  (`ndlea-edo-igbanke-farms`).
- Country: the CFI id from `src/data/countries.json` (`ng`, `us/ca`).
- Coordinates for the most precise named place, with `precision`.
- Text, figures, citations and status per docs/WRITING.md; `status: verified` only when every fact was
  read in an opened source.
- `open_questions` (docs/WRITING.md §9): what the sources leave open (figures not given, an unclear
  date, no arrests or outcome reported, sources that disagree, a separate event left out, a single
  source), one plain sentence each. The automated research records these too; reports written by
  hand should not have fewer.

## 5. Archive

```bash
node scripts/archive-media.mjs <slug> <slug> …
```

Keeps local copies of every picture on the sources and media pages (`archive/`, not committed or
published). Then

```bash
node scripts/archive-sources.mjs <slug> <slug> …
```

adds the Wayback Machine's snapshot of each source and media page as `archived:`. Pages it has no
copy of are listed; `--save` asks it to save them (only when the user has agreed to submit pages).

Every new report must include `created_by_model` with your actual runtime model identifier
as `provider/model#effort` when available (for example `alibaba-token-plan/qwen3.8-flash#xhigh`,
where `xhigh` is the reasoning effort setting). Include a known effort even if configured
separately from the model; omit only the suffix when effort is unavailable. Do not omit the
field, guess an effort or copy a sample model.
Use `unknown` only if the runtime does not disclose its identity or a historical creator was
not recorded, and state that limitation. Updates retain the original creator and do not record
update-model metadata. See `docs/WRITING.md` for the provenance rules.

## 6. Check and commit

```bash
npm run build
```

It runs the tests (sources, citations, folder, slug, country) and builds every page. Read the new
report pages in `dist/report/<slug>/index.html` if anything looks off.

Others may work in this repository at the same time: check `git status`, commit only your own new or
changed report files and images, one commit per run:
`Reports: <country> <type>, … (n new)`.

Do not run `sync-tracker.mjs` or commit `src/data/tracker.json` in the CFI checkout; that sync is
handled separately.

Finish with a short list: each new report's title, country, date and sources, and any candidate you
dropped and why.
