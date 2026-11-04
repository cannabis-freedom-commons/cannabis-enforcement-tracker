# Duplicate report review — 4 November 2026

Initial whole-collection scan: **5,210 reports**, **769 candidate pairs**. Three redundant records consolidated; two repeated components removed from a roundup. Seven surviving reports changed. No commit or push was requested.

This is a whole-collection automated screen with targeted source verification, **not a completed source-by-source adjudication of all 769 pairs**. The [pair ledger](DUPLICATE-REVIEW-2026-11-04.tsv) preserves every initial candidate and clearly separates verified decisions from unchecked scan leads. There are three highlighted unresolved pairs; other unchecked leads also remain open to review. No similarity score was treated as permission to delete a report.

## Consolidated records

| Retired slug / code | Canonical slug / code | Reason |
| --- | --- | --- |
| chauddagram-111kg-cannabis-four-arrests / TY1RDR | chauddagram-baburchi-bazar-111kg-two-arrests / C4NGE4 | Same 111 kg truck interception; the two liquor-only arrests were incorrectly included. |
| tobruk-400kg-hashish / 8JVT39 | libya-tobruk-hashish-2026 / 12B6H0 | Same 400 kg seizure, house, date, detainee nationalities and valuation. |
| obioha-singapore-cannabis-death-sentence-2008 / 02WRPN | chijioke-obioha-hanged / 1RBAYT | One case from arrest and sentence through final appeal and execution. |

Canonical records were chosen by existing tracker addition times, not incident dates. All original canonical codes, addition metadata and creator attribution were preserved. Retired slugs and codes point directly to canonical slugs. All source URLs and media entries from the retired records survive; no cover assets or evidence archives were deleted. No seizure quantities were added together across duplicate accounts.

## Partial overlap and incidental corrections

- Kashmir roundup 4TD728 now covers six Anantnag/Pulwama arrests and 3.347 kg of stated weights, plus five unweighed sticks. Pethkoot 0F22T4 retains its two arrests and 1.030 kg; Tral P0RM9H retains its one arrest and 1.070 kg, with the separate 30 g of unconfirmed material in prose. Both individual reports gained the opened Kashmir Images account. The existing roundup photograph remains, identified as a picture accompanying the broader nine-arrest roundup.
- Huahine sentence 2F6WGF remains separate from August customs seizure 3BNDEF. Its opened source dates the arrests to April and does not state 177 sticks. Its 500,000 CFP reseller fine was a prosecutor's request, not a reported court order. These unsupported details and the false August connection were removed.
- Obioha retains early reports' conflicting ages/nationalities and the withdrawal-date discrepancy in open questions; status is disputed. The unsupported January 2009 appeal-filing month is explicitly recorded as unconfirmed rather than silently certified.
- Agrajatra was recovered by direct HTTP after web-reader failure. Its current text agrees with BD24Live on the cannabis suspects' names/ages and the joint team; stale contradictory questions were corrected. Printed incident months still conflict and remain open.

## Evidence opened

- [BD24Live](https://www.bd24live.com/bangla/811689), [Agrajatra](https://www.agrajatrapratidin.com/country/news/28564) (direct HTTP recovery).
- [Middle East Online](https://www.middle-east-online.com/en/libya%E2%80%99s-latest-drug-bust-nets-400-kg-hashish) (direct HTTP recovery), [Reuters via Investing.com](https://www.investing.com/news/world-news/libya-seizes-nearly-1-million-of-hashish-in-east-4943973).
- [CNB execution notice](https://www.cnb.gov.sg/mediaroom/news/execution-of-convicted-drug-trafficker-2016-2/), [2008 High Court judgment](https://www.elitigation.sg/gdviewer/s/2008_SGHC_243), [2016 appeal judgment](https://www.elitigation.sg/gd/gd/2016_SGCA_63/pdf), [Straits Times](https://www.straitstimes.com/singapore/two-drug-traffickers-hanged-for-their-offences), [Amnesty](https://www.amnesty.or.jp/en/news/2009/0113_973.html), [June 2007 dispatch](https://ccguide.org/news/shownewsarticle.php?articleid=12609), plus the SGForums and two 420 Magazine reprints recovered by direct HTTP.
- [Kashmir Images](https://thekashmirimages.com/2026/10/31/9-drug-peddlers-arrested/).
- [Tahiti Infos](https://www.tahiti-infos.com/Plus-professionnel-pour-planter-du-cannabis-que-des-pasteques_a239502.html), [TNTV](https://www.tntvnews.pf/polynesie/faits-divers/une-demi-tonne-de-paka-saisie-a-huahine/).
- [Modern Ghana](https://www.modernghana.com/news/1509442/police-destroy-11-cannabis-farms-in-volta-region.html), [Graphic Online](https://www.graphic.com.gh/news/general-news/ghana-news-police-destroy-2-marijuana-plantations-in-peki-dzake-forest.html): retain separate July 9 and July 16 operations.
- [October Aceh account](https://www.antaranews.com/berita/5816651/bnnp-aceh-dan-tim-gabungan-ungkap-temuan-ladang-ganja-18-hektare), [November Aceh photo captions](https://www.antaranews.com/foto/5827183/tim-gabungan-musnahkan-ladang-ganja-seluas-18-hektare-di-aceh-utara-tiga-orang-ditangkap): different detainees; exact field overlap unresolved.

Daily Naya Diganta, Bangladesh Pratidin, Al-Monitor, Daily Excelsior, Brighter Kashmir and the RRI follow-up could not be reopened through the web reader. Retained citations were not represented as fresh verification. Direct HTTP recovery succeeded for the other reader failures identified above. Local media archiving completed for all seven changed records; source archive lookup added six snapshot references and found no snapshot for 21 pages. A missing snapshot is not verification or a successful archive.

## Validation and rescan

- Codes checked without reassignment; original identity metadata compared against Git.
- Previous source/media inventories compared against all three merged survivors; all retained.
- Full build and tests: 5,365 tests passed, none failed or skipped.
- Default load test: 5,000 synthetic reports; 38.6 seconds; home 142 KB, representative report 33 KB, shared index 2,259 KB. The existing build emits a large-client-chunk warning.
- Generated HTML, retired slug/code redirects and unique JSON-index entries checked for every merge. Browser country-filtered searches for Bangladesh 111, Libya 400 and Singapore Obioha each found the canonical report once; all six retired slug/code URLs also navigated to their live targets. External requests were blocked during this local browser check, so map tiles and remote media loading were not validated.
- First rescan: 5,209 live reports and 764 candidates. Two reports were added concurrently while three were retired; there were no new candidate pairs compared with the initial scan. Existing unrelated work was left alone.

Final rescan: **5,215 live reports and 764 candidate pairs**. Across the initial scan and concurrent additions, **5,218 distinct report records were screened** (three were retired). Eight reports entered during the audit; no new candidate pairs appeared. The build validates a fixed snapshot while intake continues.

Remaining leads are retained for review; this review makes no duplicate-free guarantee.
