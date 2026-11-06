// What an existing report lacks against today's writing rules (docs/WRITING.md), which changed
// over time: older reports predate photos with direct addresses, archived copies, structured
// details and their sources, open questions… Used by scripts/report-gaps.mjs and the
// enrich-report skill. Each gap is something a writer can act on; none is proof of an error.
export const GAPS = {
  "no-coordinates": "No coordinates: the dot sits at the country's middle",
  "coarse-place": "Location only to region or country precision",
  "single-source": "Only one source: look for a second account or the agency's statement",
  "unarchived-source": "A source without an archived copy",
  "undated-source": "A web source without its publication date",
  "no-media": "No photo or video of the event",
  "media-no-url": "A photo or video without its direct media_url",
  "unarchived-media": "A media page without an archived copy",
  "no-details": "No structured details (agencies, products, setting, case stage…)",
  "unsourced-figure": "A figure with no data_sources entry",
  "unsourced-detail": "A detail with no data_sources entry",
  "not-verified": "Status reported: not every fact was checked against an opened source",
  "no-open-questions": "No open questions recorded",
  "no-tags": "No tags",
};

const UNQUOTED = new Set(["part_of", "related", "date_precision", "region"]);

// the gaps of one report, from its parsed front matter
export function reportGaps(data) {
  const gaps = [];
  const sources = data.sources ?? [];
  const web = sources.filter((s) => s.url);
  if (!data.coordinates) gaps.push("no-coordinates");
  else if (["region", "country"].includes(data.precision)) gaps.push("coarse-place");
  if (sources.length < 2) gaps.push("single-source");
  if (web.some((s) => !s.archived)) gaps.push("unarchived-source");
  if (web.some((s) => !s.date)) gaps.push("undated-source");
  const media = data.media ?? [];
  if (!media.length && !data.cover) gaps.push("no-media");
  if (media.some((m) => !m.media_url)) gaps.push("media-no-url");
  if (media.some((m) => !m.archived)) gaps.push("unarchived-media");
  const cited = data.data_sources ?? {};
  if (!data.details) gaps.push("no-details");
  else if (Object.keys(data.details).some((k) => !UNQUOTED.has(k) && !cited[`details.${k}`])) gaps.push("unsourced-detail");
  if (Object.keys(data.figures ?? {}).some((k) => !cited[`figures.${k}`])) gaps.push("unsourced-figure");
  if (data.status === "reported") gaps.push("not-verified");
  if (!data.open_questions?.length) gaps.push("no-open-questions");
  if (!data.tags?.length) gaps.push("no-tags");
  return gaps;
}
