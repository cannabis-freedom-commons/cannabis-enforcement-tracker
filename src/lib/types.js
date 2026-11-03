// The kinds of enforcement a report can be about, in the order they are listed and filtered.
// Each has a colour for its marker on the map: warm, hostile tones for what the authorities do to
// land and people, cooler ones for the courts. No green: green is the Landrace.wiki layers (living
// landrace cannabis), so the two never read alike.
export const TYPES = {
  eradication: { label: "Eradication", plural: "Eradications", color: "#e8743b", help: "Fields or plants destroyed" },
  seizure: { label: "Seizure", plural: "Seizures", color: "#e3b341", help: "Cannabis, money or property taken" },
  raid: { label: "Raid", plural: "Raids", color: "#e0565b", help: "Homes, shops, clubs or farms searched" },
  arrest: { label: "Arrest", plural: "Arrests", color: "#c23a4a", help: "People detained" },
  prosecution: { label: "Prosecution", plural: "Prosecutions", color: "#a56cc1", help: "Charges and trials" },
  sentence: { label: "Sentence", plural: "Sentences", color: "#6b8fd6", help: "Prison terms, fines, caning" },
  execution: { label: "Execution", plural: "Executions", color: "#f2f0e9", help: "People put to death for cannabis" },
  death: { label: "Death", plural: "Deaths", color: "#8f8d85", help: "People killed in enforcement or in custody" },
};
export const TYPE_IDS = Object.keys(TYPES);

// How well a report's coordinates pin the place, from the spot itself to the whole country.
export const PRECISION = {
  exact: "the spot itself",
  locality: "the village, town or city",
  district: "the district or municipality",
  region: "the province, state or region",
  country: "the country",
};
// How the report page says it, beside its map: a short label and what the dot stands for.
export const PRECISION_INFO = {
  exact: { label: "Exact spot", text: "The dot marks the place itself." },
  locality: { label: "Village, town or city", text: "The dot marks the settlement the sources name, not the exact spot." },
  district: { label: "District", text: "The dot marks the middle of the district or municipality the sources name; it happened somewhere in it." },
  region: { label: "Province or region", text: "The dot marks the middle of the province, state or region the sources name; it happened somewhere in it." },
  country: { label: "Country only", text: "The sources name no closer place; the dot marks the middle of the country." },
};

// What has been checked: `verified` means the facts were read in at least one source that was
// opened; `reported` that they are taken from a report not yet checked; `disputed` that sources
// contradict each other or the authorities' account is contested.
export const STATUS = {
  verified: "Checked against the sources",
  reported: "Not fully checked against the sources",
  disputed: "Sources disagree",
};

// Each type's map marker: a diamond in its colour with a small pictogram, as on war maps. The same
// SVG draws the dots on the map (src/components/WorldMap.astro) and the legend.
const GLYPHS = {
  eradication: '<path d="M16 9.2c2.1 2.9 4.2 4.6 4.2 7.6a4.2 4.2 0 0 1-8.4 0c0-1.7.8-2.8 1.7-3.6.2 1.2.8 2 1.7 2.2-.4-2.3.1-4.5.8-6.2z" fill="currentColor"/>',
  seizure: '<rect x="11" y="12" width="10" height="9" rx=".6" fill="none" stroke="currentColor" stroke-width="1.7"/><path d="M11 15.2h10M16 12v3.2" stroke="currentColor" stroke-width="1.7"/>',
  raid: '<path d="M18.5 10.5h3.2v11h-3.2" fill="none" stroke="currentColor" stroke-width="1.7"/><path d="M10 16h6.5m-2.6-2.7 2.7 2.7-2.7 2.7" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/>',
  arrest: '<circle cx="12.8" cy="17.6" r="2.7" fill="none" stroke="currentColor" stroke-width="1.7"/><circle cx="19.2" cy="17.6" r="2.7" fill="none" stroke="currentColor" stroke-width="1.7"/><path d="M12.8 14.9 16 12.2l3.2 2.7" fill="none" stroke="currentColor" stroke-width="1.5"/>',
  prosecution: '<path d="m11.3 13.6 3.1-3.1 4.2 4.2-3.1 3.1z" fill="currentColor"/><path d="m16.8 16.8 4.4 4.4M10 22.2h6.4" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>',
  sentence: '<rect x="11" y="10.5" width="10" height="11" fill="none" stroke="currentColor" stroke-width="1.7"/><path d="M14.4 10.5v11M17.6 10.5v11" stroke="currentColor" stroke-width="1.5"/>',
  execution: '<path d="m12 12 8 8m0-8-8 8" stroke="currentColor" stroke-width="2.3" stroke-linecap="round"/>',
  death: '<path d="M16 10v12M12.4 14h7.2" stroke="currentColor" stroke-width="2.1" stroke-linecap="round"/>',
};
const light = (hex) => { const n = parseInt(hex.slice(1), 16); return ((n >> 16) * 299 + ((n >> 8) & 255) * 587 + (n & 255) * 114) / 1000 > 150; };
export function typeIcon(type, size = 32) {
  const { color } = TYPES[type];
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="${size}" height="${size}" style="color:${light(color) ? "#0c0c0b" : "#fff"}"><path d="M16 1.5 30.5 16 16 30.5 1.5 16z" fill="${color}" stroke="#0c0c0b" stroke-width="1.6" stroke-linejoin="round"/>${GLYPHS[type]}</svg>`;
}
