// The figures a report gives, as short labels in reading order. Shared by the report pages (built
// from the files) and the draft preview (drawn in the browser from the AI's answer).
const fmtNum = (n) => n.toLocaleString("en-GB", { maximumFractionDigits: 1 });
const fmtMoney = (m) => `${m.currency} ${fmtNum(m.amount)}`;

// Data stays in kilograms; choose a readable unit without rounding small seizures
// to zero. Significant digits also suppress floating-point conversion noise.
export function weightParts(kg) {
  if (kg >= 1000) return { value: kg / 1000, unit: "t" };
  if (kg >= 1 || kg === 0) return { value: kg, unit: "kg" };
  if (kg >= 0.001) return { value: kg * 1000, unit: "g" };
  return { value: kg * 1e6, unit: "mg" };
}
export const formatWeightNumber = (value, precision = 12) => value.toLocaleString("en-GB", { maximumSignificantDigits: precision });
export function formatWeight(kg, precision = 12) {
  const { value, unit } = weightParts(kg);
  return `${formatWeightNumber(value, precision)} ${unit}`;
}

// the figures a report gives, in reading order, as { key, label, value }
export function figureRows(f = {}) {
  const rows = [];
  if (f.arrested != null) rows.push({ key: "arrested", label: "Arrested", value: fmtNum(f.arrested) });
  if (f.charged != null) rows.push({ key: "charged", label: "Charged", value: fmtNum(f.charged) });
  if (f.plants_destroyed != null) rows.push({ key: "plants_destroyed", label: "Plants destroyed", value: fmtNum(f.plants_destroyed) });
  // with its unit, like the weight, so its tooltip can give it in acres
  if (f.hectares != null) rows.push({ key: "hectares", label: "Area", value: `${fmtNum(f.hectares)} ha` });
  if (f.kg_seized != null) rows.push({ key: "kg_seized", label: "Cannabis", value: formatWeight(f.kg_seized) });
  if (f.prison) rows.push({ key: "prison", label: "Prison", value: f.prison });
  if (f.fine) rows.push({ key: "fine", label: "Fine", value: fmtMoney(f.fine) });
  if (f.deaths != null) rows.push({ key: "deaths", label: "Deaths", value: fmtNum(f.deaths) });
  if (f.value) rows.push({ key: "value", label: "Value (official estimate)", value: fmtMoney(f.value) });
  return rows;
}
