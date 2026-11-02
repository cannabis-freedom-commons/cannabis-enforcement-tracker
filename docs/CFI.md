# The tracker and the Cannabis Freedom Index

The Cannabis Freedom Index (cannabisfreedom.fyi, checkout `../cannabis-freedom-index`) records what
each country's law allows and how it got there: its history of laws, rulings and treaties. The
tracker records how the law is applied: raids, seizures, eradication, arrests, prosecutions and
sentences. Country ids are the CFI's.

## Substance scope

The tracker covers enforcement involving cannabis plants and cannabis products such as marijuana,
hashish, resin, oil and edibles. **Synthetic cannabinoids and their precursors are out of scope.**
Do not add synthetic-only cases, including products described as "synthetic cannabis", Spice, K2
or bonzai, or hemp sprayed or infused with synthetic cannabinoids when the hemp is only their
carrier. These cases are outside this tracker's scope, not referrals to the CFI history.

A mixed event can qualify when opened sources establish a separate cannabis enforcement component.
Focus the report on that component; other substances may be mentioned to explain the same event,
but never include their quantities in cannabis figures or attribute combined drug totals to cannabis.
Determine the substance from the opened evidence, not a headline or trade name alone. If the
cannabis component cannot be established, do not draft a cannabis report.

## What goes where

**Comes here (a report):** a single enforcement event since 2000 that changes no law: an operation,
a raid, a seizure, fields destroyed, an arrest, a trial, a sentence, or an appeal that only changes a
sentence.

**Goes to the CFI history** (its `changes.yaml`, see its `docs/DATA-MAINTENANCE.md`, "Enforcement events
go to the tracker"):

- anything that changes the law or a country's score, and court rulings that set a precedent;
- events before 2000;
- famous and landmark cases (a celebrity or politician, a case that started a reform, the first
  sentence under a new law);
- national statistics and the overall picture (yearly figures, a crackdown or an amnesty announced
  as policy);
- executions and death sentences (the CFI keeps each country's death-penalty record);
- anything a CFI atlas entry lists or a did-you-know fact names.

Found a change in the law while looking for reports? Add it to the CFI history (its
`expand-country` skill), not here.

## How the two are connected

- `npm run countries:sync` copies the CFI's places, scores and summaries for the CFI widget on
  country pages (README, "Country ids and the CFI widget").
- In the CFI, `node scripts/sync-tracker.mjs` copies a line per report into its `src/data/tracker.json`
  for the **Enforcement** section of each profile. Run it there after reports are committed here.
- `scripts/import-cfi-history.mjs` moved the CFI's existing enforcement entries here (416 in October
  2026), keeping each entry's id as the slug and in `cfi_id`; the CFI's `scripts/move-to-tracker.mjs`
  then took them out of its history and redirects their old `/history/<id>/` addresses here. Running
  the import again only adds entries that are not here yet.
