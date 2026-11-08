# Writing a report

A report tells one enforcement event, or one campaign, so that a reader understands in a minute what
happened to people, what it cost, and why it matters. The data format is in the README; this is about
what goes into a report and how it reads.

## 1. One report, one event

A report is about **one** raid, one campaign phase, one trial or one execution. Everything in it, and
every source it cites, must be about that event.

- **Sources about the same event only.** The agency's statement, the reports on it, the court's
  judgement, reactions *to this event*. An article about an earlier phase of the same operation, an
  older case or the general situation is not a source for this report, even if it adds good context.
  (The first version of the Paraguay report cited a 2025 article on phase 53 for a phase-57 report;
  that was wrong.)
- **A case that runs for months** (arrest → trial → sentence) is one report dated at its latest
  step, and may cite the earlier coverage of the same case. The LBC piece on Bella Culley's pre-trial
  hearing is a fair source for the report on her sentence.
- **Context that needs another event** gets its own report, and the two are found through the
  country page and "Related reports". Don't fold it into this one.
- **Campaign totals** (a quarter's seizures, a season's eradication) are one report when an
  authority publishes them as one result. Name the period in the title.
- **Useful finds for another report belong there.** While researching one event, you may encounter
  another source, a new detail, a correction or an answer to an open question for a case already
  recorded. Open the source, find the matching report in the coverage index, and read the report
  before changing it. Confirm the match from the event, people, place and dates; a similar headline
  or the same country is not enough. Make the supported update in the same run without waiting for
  a separate request. A corroborating source strengthens existing sentences with citations; a
  genuinely new detail gets its own cited sentence. Resolve answered open questions and record
  remaining discrepancies. Preserve the stable slug, earlier evidence and original creator, and
  avoid retelling facts already present. Keep the other event out of the report you started with.
  Archive and validate every changed report, and list these updates in the final response.

Automated research uses `report_updates` for supported updates to other reports, within the
submission contract's source and outcome limits. Never insert an unrelated source or fact into
the main report to satisfy those limits; explain a useful follow-up that the contract cannot
represent in the decision explanation instead.

## 2. Open every source

Only facts from sources you have **opened and read** go in. A search-result snippet, an AI summary
or a headline is not a source.

- Prefer, in this order: the court's or agency's own statement; reporting that names its source;
  wire services; other outlets. Two independent sources are better than one for anything disputed.
- **Note who is speaking.** Most figures are the authorities' own: "the agency valued them at 9.6
  million pesos", not "plants worth 9.6 million pesos". Street values are official estimates and run
  high; mark them `value` and say whose they are.
- **When numbers differ**, use the final or official one and say that another was reported
  ("On 24 August, with the operation still under way, the count stood at 145 hectares"). If sources
  truly contradict each other, set `status: disputed` and give both.
- Check the article's **own date**, not the date in the page header. News sites show today's date
  everywhere; many "2026" pages are years old.
- A headline that disagrees with its own text (ten arrests in the title, eleven named in the body):
  use the text and count yourself.

## 3. How it reads

Plain English, short sentences, the same style as the Cannabis Freedom Index.

- **Lead with people.** Who was arrested, jailed, executed, lost a field, a home or a living. Then
  the numbers. Then the context.
- **Say what the authorities did, in their words where it matters**, and what they did not say
  ("No suspects were arrested." "The report names no arrests.").
- Explain local terms once: "Senad, Paraguay's anti-drug agency"; "a barangay, the smallest unit of
  local government".
- No adjectives the source doesn't support (no "massive", "brutal", "notorious") and no opinion of
  ours. Critics and the accused get a voice through quotes from the sources, briefly.
- No lists of law sections. Say what the law does: "importing more than 500 grams carries a
  mandatory death sentence".
- **Length:** 150–400 words. Two or three short sections (`##`) at most. The summary is two
  sentences that stand on their own in a list and in the JSON.
- **Title:** what happened, in under 90 characters, with the key number if there is one.
  "48,000 cannabis plants burned in a Kalinga communal forest", not "PDEA conducts operation".

## 4. Citations

Every sentence that states a fact ends with its source: `[@1]`, or `[@1,2]` for two. Every source
must be cited at least once; the build stops otherwise. Use the source's title and publisher as
published, with `language:` if it is not English.

## 5. Figures

Only numbers a source states. Convert units where the field asks for it (pounds → kg, acres →
hectares), keep the original in the text ("63,204 pounds (about 28.7 tonnes)"). A figure for people
(`arrested`, `charged`, `deaths`) counts the people the source names or counts for **this** event.
Leave a field out rather than guessing.

Keep `kg_seized` in kilograms without rounding away small quantities: 3.86 g is `0.00386`.
The site chooses mg, g, kg or tonnes for display; the stored unit stays kilograms.

Apply the substance scope in `docs/CFI.md`: synthetic cannabinoids and their precursors are not
cannabis figures. In mixed events, use only the separately supported cannabis weight; omit a
combined weight that cannot be separated, and explain that limitation in `open_questions`.

### Structured details and their sources

`kg_seized` and `plants_destroyed` stay the totals (seized or destroyed). Where a source splits
them, add `plants_seized` (taken away) or `kg_destroyed` (burned). The court outcome and harm have
their own counts: `convicted`, `acquitted`, `sentenced`, `released`, `injured`; and `sites`,
`seeds`, `vehicles` and `cash` (money confiscated) where stated. An omitted figure means "not
reported"; write `0` only when a source says none.

`details` records who acted, what was taken, where and how far the case went, with ids from
`src/data/vocabularies.json` and `src/data/agencies.json`:
`agencies` (registry id, or the name as published), `agency_types`, `products` (with `kg` or
`plants` per product where stated), `activity`, `setting`, `environment`, `charges` (as stated),
`case_stage`, `sentence_types`, `force`, `period` (start and end of a multi-day operation),
`date_precision`, `region`, `route` (`from`/`to` country ids, `crossing`), `part_of` (a campaign
report's slug) and `related` (slugs of other stages of the same case).

Every figure and every detail except `part_of`, `related`, `date_precision` and `region` needs a
source that states it: list the sources' numbers in `data_sources` (`figures.arrested: [1]`,
`details.agencies: [1, 2]`), only sources the text cites. A value no source supports stays out.
Never put a person's name or other identifiers in `details`; counts and outcomes only. A later
stage of a case replaces the details it changes (`case_stage`, `sentence_types`) and keeps the rest.

## 6. Place

Give `coordinates` for the most precise place the sources name, and say how precise with
`precision` (`locality`, `district`, `region` …). Coordinates of a town or district centre from
OpenStreetMap or Wikipedia are fine; don't guess a spot in a forest.
Run `node scripts/check-coordinates.mjs reports/<year>/<month>/<slug>.md` after setting coordinates.
It flags points outside the mapped country and suggests sign/order mistakes for review. Legitimate
offshore events, small islands and simplified borders can also be flagged; verify the source
rather than automatically moving or rejecting a point. An inside result does not verify the
named locality. See [coordinate checks](COORDINATES.md) for the full audit and limitations.

## 7. Pictures

A photo of the action makes a report far stronger: fields burning, agents in a plantation, a raid,
a courtroom. But most photos in news articles belong to the newspaper or the agency, and **may not be
copied** to this site.

- **`cover`: a copy on this site**, only when the licence allows it:
  - Wikimedia Commons files, Creative Commons licences (CC BY, CC BY-SA, CC0);
  - works with no copyright: US federal agencies (DEA, CBP, US Forest Service), Philippine
    government works (PDEA, PNP; RA 8293 §176), and other governments whose law says so;
  - photos released for reuse by the agency itself, with that release linked.
  Save it in `src/assets/reports/<slug>.jpg` (1200–1600 px wide), fill in `alt` (what is in the
  picture), `credit` (the photographer or agency, "via" the outlet), `licence` and `url` (where it
  and its licence can be checked). Record `media_url` as the original direct image address too.
- **`media`: links**, for every other good photo or video of the event: the page it is on, its
  publisher, and a caption saying what it shows. For **every newly collected photo or video**, also record
  `media_url`: the absolute HTTP(S) address of that individual photo or video. For photos,
  extract it from the opened page's image markup, `srcset`, metadata, or captured
  `source_images[].src`. For videos, use the published file/stream URL or dedicated video-host page. Keep `url` as the
  article/source page; keep it separate from the media address. Use one entry per photo or video, even
  when several are on the same page, and preserve meaningful media query parameters. List each
  picture once: an agency handout republished by several outlets is the same photo (prefer the
  agency's page or the first outlet); `node scripts/duplicate-media.mjs` finds repeats. Never
  guess an address or substitute a news article URL. If no address can be recovered, explain the
  missing media in `open_questions` instead of inventing a media entry. A media URL does not grant
  permission to copy the asset: the full photo is never copied into the site, only a 320px
  thumbnail for lists and grids (`public/media/`, by `scripts/media-manifest.mjs`). Old entries without `media_url`
  remain valid.
  Under the text, "Photos and video" shows each photo with a `media_url` in a grid, loaded through
  DuckDuckGo's image proxy so readers never contact the publisher (click to enlarge); report lists show
  the first photo as a thumbnail; plays a direct video file or a YouTube/Vimeo video in
  place once the reader clicks its poster (nothing loads from the video's site before), each with its caption and source; a photo the publisher no longer serves, and any other
  entry, stays a link (`src/lib/media.js`). After adding photos, run `node scripts/media-manifest.mjs`: it records
  each photo's size and placeholder in `src/data/media.json` and makes its thumbnail; the original is not kept.
- Never publish a newspaper's photo, a stock photo, or a picture of a different event or of a building
  sign. A photo must show *this* event.
- **Leave out generic pictures.** Many outlets put a stand-in picture on every crime story: a judge's
  gavel, handcuffs, prison bars, a police sign or car, a prosecutor's folder, a city or section
  banner ("Adana Haberleri", "Jhansi"), a cartoon or AI drawing, a picture of drugs from another
  story. Signs of one: the caption only repeats the headline or the outlet's name; the outlet calls it
  a file, archive, symbolic or illustrative picture; the image address names a city, a section or a
  video thumbnail (`local-news/adana-960x540`, `maxresdefault`), or its upload folder is years older
  than the event. Keep such a picture only when its caption or the article ties it to this event
  (police handout of the seizure, the accused, the place). If the caption would have to say the
  picture is not known to show the event, leave it out instead of describing the doubt: the
  submission service drops new photos with such captions automatically. Photos already in reports
  are not removed by this rule.
- **Archive the pages.** Pages disappear, and they are the evidence. After writing a report, run
  `node scripts/archive-sources.mjs <slug>` to add the Wayback Machine's copy of each source and
  media page as `archived:` (`--save` asks it to save pages it lacks).

## 8. Status

- `verified`: every fact was read in a source that was opened. The default for a finished report.
- `reported`: one or more retained claims have not been checked against an opened source.
- `disputed`: sources contradict each other, or the official account is contested.

Automated reports use the same definitions: the service assigns `verified` after every retained
fact passes its quotation check against opened/captured sources. It keeps `reported` when a
retained claim could not be checked, such as a source that could not be reopened or recovered
from a capture. A missing second source or an unanswered question does not by itself prevent
verification. Existing disputed reports stay disputed; a checked addition does not certify
older unchecked prose. Corrected titles or summaries remain `reported` pending a full check.

## 9. Open questions

What the sources leave open goes in `open_questions` in the front matter, not into the text: the
report says what is known, the questions say what is not. The report page lists them under the text
as "Open questions". One plain sentence each, standing on its own, no citations:

- **Missing figures:** no count of plants, no weight, area, value or number of people
  ("No figure for the plants destroyed or the area cleared was published").
- **Dates:** only the publication date is known, or the operation may have begun earlier
  ("The article gives no separate date for the raid; 12 March is its publication date").
- **What happened next:** no arrests reported, charges or a court outcome not yet known
  ("No one was named or charged in these reports").
- **Disagreements:** sources give different numbers or accounts; say which one the report uses.
- **Left out:** a separate event in the same article that belongs to another report.
- **Sources:** only one source could be found, a source is a reprint of another, a page could not be
  opened again or is in a language you could only partly read.

```yaml
open_questions:
  - No figure for the number of plants destroyed or the area cleared was published.
  - The same roundup reports a separate Kurukshetra police drive, a different event left out here.
```

Leave the field out when nothing is open. A question is never a place for a guess: if a source
answers it, the answer goes in the text with its citation.

## 10. Required model provenance

Every report's front matter must contain `created_by_model`, naming the exact model that wrote
it, including the provider and the actual reasoning effort setting whenever available. Use
`provider/model#effort`, for example:

```yaml
created_by_model: alibaba-token-plan/qwen3.8-flash#xhigh
```

Here `alibaba-token-plan` is the provider, `qwen3.8-flash` is the model, and `xhigh` is the
reasoning effort setting (also called a reasoning variant). Preserve the exact effort used
by the writing run, including when it is configured separately from the model identifier;
do not drop it from the attribution. If the model is known but its effort is not available,
record `provider/model` without a suffix. Never guess an effort or infer it for older work.

Agents writing Markdown fill this in themselves; the submission service assigns it from the
actual OpenCode run configuration. It is required by both the schema and the report tests.
Never claim another model, copy an example's attribution, or infer an old model from today's
configuration. For historical reports without recorded attribution, or when the runtime does
not disclose its identity, write `created_by_model: unknown` and explain the limitation.
Preserve the original creator when updating a report. Do not add update-model metadata. The report page shows recorded attribution or "not recorded".

## 11. Addition and update times

Keep the incident `date` separate from `added_at`, the UTC timestamp when a report first enters
the tracker. `npm run codes -- <new/updated report paths>` assigns addition metadata automatically;
the submission service assigns it to new accepted drafts itself. Do not use an article publication
date or submission receipt time as the tracker addition time. Preserve the original `added_at` and
`added_at_source` on edits, renames and date-folder moves.

Existing reports have their Git addition times written directly into their front matter.
Git times are the relevant tracker times; `added_at_source: git` records their origin.
Bulk additions share a timestamp. Missing or shallow history leaves unknown times unset.

Last-update times are derived from committed Git content changes, ignoring addition-timestamp
metadata migrations. Do not store `updated_at` in report files. A report without a later content
change has no update label. `npm run times` persists recoverable addition times. Report details show
relative addition/update times, with exact UTC timestamps on hover; lists show only incident dates.

## 12. Permanent report code

Every report must store `code` as a six-character Crockford base32 string, without the `CET-`
display prefix. Assign it with `npm run codes -- <new/updated report paths>` before the build
and commit. The script checks existing codes and resolves collisions; do not invent or copy one.
The backend assigns codes automatically using the same slug hash and collision rule. Preserve
the original code when updating, renaming or moving a report, so its short links remain stable.
The schema and tests reject missing, invalid or duplicate codes; the build never assigns them.

## Checklist

- [ ] Required `code` persisted with `npm run codes -- <report paths>`; existing code preserved.

- [ ] Required `created_by_model` records the actual provider, model and effort when available; original creator preserved on updates.
- [ ] One event; every source is about it.
- [ ] Every source opened; its own publication date checked.
- [ ] Every factual sentence cited; every source cited.
- [ ] Figures only as stated, units converted, whose estimate said.
- [ ] Coordinates with precision.
- [ ] What the sources leave open listed in `open_questions` (§9).
- [ ] A cover only with an open licence; other photos as `media` links; every newly collected photo or video has its direct `media_url` and source-page `url`.
- [ ] `archived` source copies where they exist (`node scripts/archive-sources.mjs <slug>`).
- [ ] `npm test` and `npm run build` pass.
