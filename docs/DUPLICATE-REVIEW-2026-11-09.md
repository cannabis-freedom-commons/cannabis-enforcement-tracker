# Duplicate report review — 9 November 2026

Whole-collection scan at the start of this pass: **7,849 reports**, **1,280 candidate pairs**. Six redundant records consolidated; one roundup had a duplicated component removed; one figure that copied another event's weight was corrected. Nine surviving reports changed. No commit was requested.

This is a whole-collection automated screen plus source checks on the fresh high-score, shared-source and shared-name pairs that were not in the [4 November ledger](DUPLICATE-REVIEW-2026-11-04.md). It is **not** a source-by-source adjudication of all 1,280 pairs. No similarity score was treated as permission to delete a report.

## Consolidated records

| Retired slug / code | Canonical slug / code | Reason |
| --- | --- | --- |
| dinteloord-garage-box-890g-cannabis-seizure / 21CEN3 | dinteloord-garage-fireworks-hash-two-arrests / AYK5EM | One garage-box search on De Vriesstraat. Police statement: 25-year-old from Dinteloord arrested 4 November 2026, 42-year-old from Steenbergen on 6 November, just over 90 g of hemp and just over 800 g of hash with the fireworks. The report date is the later arrest. Weights were not added. |
| uae-rak-network-20-arrested-marijuana-hashish / NV6BQA | uae-ras-al-khaimah-20-arrests-captagon-drugs-2026 / 3AKBB3 | One 6 November 2026 announcement: 20 suspects, 200,000 Captagon pills, more than 30 kg of other drugs including marijuana and hashish, through Ras Al Khaimah, with Iraq. No separate cannabis weight, so none was recorded. |
| egypt-school-bus-drivers-31-positive-hashish-tramadol / AMT282 | egypt-school-bus-drivers-hashish-screening / DPA5M8 | One school-year screening. Youm7 on 26 October: 3,660 drivers and 21 hashish positives in the first six weeks. The fund on 8 November: 4,510 drivers, 28 hashish positives and 3 tramadol in the first two months. The later total replaces the interim count. The file moved to `reports/2026/11/` with the same slug and code. |
| isle-of-man-boy-shayne-five-sentenced / V0BCSQ | isle-of-man-boy-shayne-dunne-12-years / FFBYKE | One Boy Shayne landing of 15.5 kg on 17 November 2023. Five island men were sentenced on 22 January 2025; Liam Dunne was jailed for 12 years in January 2026. The 15.5 kg is recorded once. |
| tubarao-gravatal-cocaine-cannabis-two-arrests / P9WVW4 | gravatal-pouso-alto-tele-entrega-arrest / T3QT3F | The roundup's only cannabis was the Gravatal tele-entrega arrest already recorded on its own. The Tubarão arrest was cocaine. After that split, no separate cannabis event remained. |
| abuja-ndlea-three-south-africans-transit-route / CH8BCT | nigeria-port-harcourt-south-african-thai-loud / JSFCMS | The briefing's only cannabis weight is Gebashe Sithandiwe Jessie's existing 32.74 kg Port Harcourt case. The other described arrest was heroin. Daily Trust was added to the Port Harcourt report. |

Canonical records are the ones first added to the tracker. Original codes, `added_at`, `added_at_source` and `created_by_model` were kept. Retired slugs and codes point directly at the surviving slugs. Source URLs and distinct media from the retired records were kept. No cover asset or evidence archive was deleted. Identical article URLs were collapsed; the Omroep Brabant Dinteloord page is one article under two slugs, and the Anadolu UAE page is one article under two hosts.

## Partial overlap and a corrected figure

- Bandipora: Kangri Chowk (Irfan Ahmad Chopan and Imran Ahmad Sheikh, FIR 181/2026, about 4.5 kg) stays in `bandipora-kangri-chowk-4-5kg-charas-two-arrested`. Garoora Crossing (Rafaqat Ali Gojar and Zakir Hussain Qureshi, FIR 72/2026, about 1.5 kg) stays in `bandipora-garoora-two-arrested-1-5kg-charas`. The Kashmir Independent Media Service roundup was opened and is cited on both for the facts it states about each stop. The 6.5 kg combined weight was removed from the Kangri report. Police still say approximately 4–5 kg for Kangri; Daily Excelsior and The Print say approximately 4.5 kg.
- Lagos snacks: `lagos-airport-uk-snacks-1-2kg-cannabis` had `kg_seized` 22.2, which is the separate 3 November courier consignment of 46 blocks. The NDLEA statement gives this cargo 1.20 kg of cannabis, hidden in snacks from the United Kingdom. The figure is now 1.2 kg and the route is United Kingdom to Nigeria. The courier report is unchanged.

## Kept separate

- Kuwait, 1 November (60 g of hashish, four cases, article 474701) and 8 November (10 g, five areas, article 475074). Different ministry rounds.
- Hong Kong airport, 1 November (22-year-old, about HK$1.3 million) and 5 November (24-year-old from Bangkok, about HK$1.9 million).
- Pune airport, 4 November (10.642 kg in noodle packets) and 5 November (3.255 kg in a trolley cavity). The shared Marathi URL was not reopened; the reports already distinguish the loads.
- Alexandria, 30 October (three people: an employee, a delivery worker and a supplier) and 7 November (two unemployed men in Montazah). Shared page URLs were not reopened.
- Nevis, 13 March: Ronald King in Hamilton and Merville Liburd in Zion. Same police day, different people and places.
- Bandipora Pethkoot, 31 October (Gh. Mohd Bhat and Manzoor Ahmad Bhat, 1.03 kg) and the 6–7 November Kangri and Garoora stops. The Pethkoot pair appeared in the rescan only after the Kangri title was rewritten.
- Lagos snacks and the 22.2 kg UK-bound courier blocks. Same NDLEA roundup, different consignments.

## Unresolved

Saudi Arabia's June and November 2026 reports of five Ethiopians executed in Asir for hashish smuggling share a headline pattern and are 135 days apart. Their sources were not opened in this pass, so they stay separate. The 4 November unresolved Aceh, Durban/Montreal and Giza pairs were not re-adjudicated.

The other candidate pairs remain unchecked scan leads. A shared roundup URL, a repeated airport or a similar headline is not identity.

## Evidence opened

- [Dutch police, Dinteloord](https://www.politie.nl/nieuws/2026/november/7/08-ruim-750-kilo-illegaal-vuurwerk-in-beslag-genomen-in-dinteloord-twee-verdachten-aangehouden.html) and [ZuidWest Update](https://www.zuidwestupdate.nl/nieuws/politie-vindt-750-kilo-illegaal-vuurwerk-dinteloord-genoeg-voor-massa-explosie/).
- [Khaleej Times](https://www.khaleejtimes.com/uae/uae-busts-international-drug-network-thwarts-attempt-to-smuggle-30kg-drugs), [Gulf News](https://gulfnews.com/uae/crime/uae-busts-international-drug-ring-arrests-20-in-major-operation-1.500738328) and [Emarat Al Youm](https://www.emaratalyoum.com/local-section/accidents/2026-11-06-1.2099961).
- [KIMS Kashmir, Bandipora](https://kimskashmir.com/over-6-kg-of-charas-like-substance-seized-in-bandipora-four-arrested/).
- [NDLEA, 8 November roundup](https://www.ndlea.gov.ng/blog/ugandan-woman-in-nigeria-for-crusade-arrested-with-87-cocaine-wraps-in-private-part).
- [Egypt fund, 8 November](http://drugcontrol.org.eg/News/Details/423) and [Youm7, 26 October](https://www.youm7.com/story/2026/10/26/%D8%B5%D9%86%D8%AF%D9%88%D9%82-%D9%85%D9%83%D8%A7%D9%81%D8%AD%D8%A9-%D8%A7%D9%84%D8%A5%D8%AF%D9%85%D8%A7%D9%86-3660-%D8%B3%D8%A7%D8%A6%D9%82-%D8%AD%D8%A7%D9%81%D9%84%D8%A7%D8%AA-%D9%85%D8%AF%D8%B1%D8%B3%D9%8A%D8%A9-%D8%AE%D8%B6%D8%B9%D9%88%D8%A7-%D9%84%D9%84%D9%83%D8%B4%D9%81-%D8%B9%D9%86/7593848).
- [BBC, Dunne sentence](https://www.bbc.com/news/articles/c93vklyz040o) and [Isle of Man Today, January 2025 sentences](https://www.iomtoday.co.im/news/men-caught-red-handed-trying-to-smuggle-ps300000-in-drugs-to-the-island-on-a-fishing-boat-758667).
- [EXTRA.SC](https://extra.sc/2026/11/06/homens-sao-presos-por-trafico-de-drogas-em-tubarao-e-gravatal/) and [Eximproviso](https://eximproviso.net/policia-militar-registra-duas-prisoes-por-trafico-de-drogas-e-apreensao-de-adolescente-na-regiao/).
- [Daily Trust](https://dailytrust.com/foreign-drug-cartels-using-nigeria-as-transit-route-ndlea/) for the Port Harcourt name and weight. It calls Thai Loud synthetic cannabis; the report keeps the NDLEA and October P.M. News account of a cannabis strain.

Archive lookup on the nine changed reports added 15 Wayback snapshots and found none for 90 pages. A missing snapshot is not verification.

## Rescan

Final rescan: **7,853 live reports and 1,275 candidate pairs**. The six consolidated pairs are gone. Ten reports were added by other work during the pass (7,849 − 6 retired + 10 new = 7,853). One new candidate appeared because the Kangri title now resembles the separate Pethkoot report; those are different people, places, dates and weights.

Across the initial scan and concurrent additions, **7,859 distinct report records were screened** (six were retired). This review makes no duplicate-free guarantee.

## Validation

`npm test` passed 8,013 tests, with 5 skipped and none failed. `npm run build` then completed. The built index has **7,855 unique reports**. Two reports were added by other work after the rescan (7,853). None of the six retired slugs is in the index, and each surviving report appears once.

Retired pages redirect to the surviving report. Checked in the built HTML, and in the browser for `/report/dinteloord-garage-box-890g-cannabis-seizure/` and `/r/21CEN3/`, both of which land on the Dinteloord garage report (CET-AYK5EM). The same redirect target is in the built HTML for the other five retired slugs and codes.

Search and filters, against the built site:

- Reports: "Dinteloord" returns the one garage report. "Ras Al Khaimah" returns that November seizure and a separate June raid; turning on Seizure leaves only the November report. "school-bus drivers hashish", "Dunne" and "Gravatal" each return one report. "Thai Loud" returns the Port Harcourt arrest and a separate Lagos and Onne ports seizure. "Garoora" and "Kangri" each return their own Bandipora arrest.
- The map, last 30 days, "Dinteloord": one report.
- Netherlands, "Dinteloord": one report.

The 5,000-report load test built 11,676 pages in 110.8 s. Home 245 KB, representative report 33 KB, shared index 2,259 KB. The build warned that some chunks are larger than 500 KB. No local dev server was running, so there was no live index to refresh.

Decisions for the pairs opened in this pass are also in `DUPLICATE-REVIEW-2026-11-09.tsv`.
