// Weights, areas, lengths and sums of money in a report's words, each explained in a tooltip by the
// glossary (glossary.js): a metric measure in US units (pounds, acres, miles, feet), any other in
// metric units (grams and kilograms, hectares and square metres, kilometres and metres),
// and money in US dollars and euros at one day's exchange rates (src/data/exchange-rates.json,
// refreshed by scripts/update-exchange-rates.mjs). "$", "Rs", "¥" and words like "rupees" or
// "pesos" are read as the currency of the report's country.
import rates from "../data/exchange-rates.json" with { type: "json" };

// the currency of each place (ISO 4217); none where the rates have no such currency
const LOCAL = {
  ad: "EUR", ae: "AED", af: "AFN", ag: "XCD", al: "ALL", am: "AMD", ao: "AOA", ar: "ARS", at: "EUR", au: "AUD", aw: "AWG", az: "AZN",
  ba: "BAM", bb: "BBD", bd: "BDT", be: "EUR", bf: "XOF", bg: "EUR", bh: "BHD", bi: "BIF", bj: "XOF", bm: "BMD", bn: "BND", bo: "BOB",
  br: "BRL", bs: "BSD", bt: "BTN", bw: "BWP", by: "BYN", bz: "BZD", ca: "CAD", cd: "CDF", cf: "XAF", cg: "XAF", ch: "CHF", ci: "XOF",
  cl: "CLP", cm: "XAF", cn: "CNY", co: "COP", cr: "CRC", cu: "CUP", cv: "CVE", cw: "XCG", cy: "EUR", cz: "CZK", de: "EUR", dj: "DJF",
  dk: "DKK", dm: "XCD", do: "DOP", dz: "DZD", ec: "USD", ee: "EUR", eg: "EGP", er: "ERN", es: "EUR", et: "ETB", fi: "EUR", fj: "FJD",
  fm: "USD", fo: "DKK", fr: "EUR", ga: "XAF", gb: "GBP", gd: "XCD", ge: "GEL", gg: "GBP", gh: "GHS", gi: "GIP", gl: "DKK", gm: "GMD",
  gn: "GNF", gq: "XAF", gr: "EUR", gt: "GTQ", gu: "USD", gw: "XOF", gy: "GYD", hk: "HKD", hn: "HNL", hr: "EUR", ht: "HTG", hu: "HUF",
  id: "IDR", ie: "EUR", il: "ILS", im: "GBP", in: "INR", iq: "IQD", ir: "IRR", is: "ISK", it: "EUR", je: "GBP", jm: "JMD", jo: "JOD",
  jp: "JPY", ke: "KES", kg: "KGS", kh: "KHR", ki: "AUD", km: "KMF", kn: "XCD", kr: "KRW", kw: "KWD", ky: "KYD", kz: "KZT", la: "LAK",
  lb: "LBP", lc: "XCD", li: "CHF", lk: "LKR", lr: "LRD", ls: "LSL", lt: "EUR", lu: "EUR", lv: "EUR", ly: "LYD", ma: "MAD", mc: "EUR",
  md: "MDL", me: "EUR", mg: "MGA", mh: "USD", mk: "MKD", ml: "XOF", mm: "MMK", mn: "MNT", mo: "MOP", mp: "USD", mr: "MRU", mt: "EUR",
  mu: "MUR", mv: "MVR", mw: "MWK", mx: "MXN", my: "MYR", mz: "MZN", na: "NAD", ne: "XOF", ng: "NGN", ni: "NIO", nl: "EUR", no: "NOK",
  np: "NPR", nr: "AUD", nz: "NZD", om: "OMR", pa: "USD", pe: "PEN", pf: "XPF", pg: "PGK", ph: "PHP", pk: "PKR", pl: "PLN", pr: "USD",
  ps: "ILS", pt: "EUR", pw: "USD", py: "PYG", qa: "QAR", re: "EUR", ro: "RON", rs: "RSD", ru: "RUB", rw: "RWF", sa: "SAR", sb: "SBD",
  sc: "SCR", sd: "SDG", se: "SEK", sg: "SGD", si: "EUR", sk: "EUR", sl: "SLE", sm: "EUR", sn: "XOF", so: "SOS", sr: "SRD", ss: "SSP",
  st: "STN", sv: "USD", sx: "XCG", sy: "SYP", sz: "SZL", td: "XAF", tg: "XOF", th: "THB", tj: "TJS", tl: "USD", tm: "TMT", tn: "TND",
  to: "TOP", tr: "TRY", tt: "TTD", tv: "AUD", tw: "TWD", tz: "TZS", ua: "UAH", ug: "UGX", us: "USD", uy: "UYU", uz: "UZS", vc: "XCD",
  ve: "VES", vg: "USD", vi: "USD", vn: "VND", vu: "VUV", ws: "WST", xa: "RUB", xk: "EUR", xn: "TRY", ye: "YER", za: "ZAR", zm: "ZMW",
  zw: "ZWG",
};
const local = (country) => LOCAL[String(country ?? "").split("/")[0]];
// a sign or word that names a kind of currency, read as the country's own where it is of that kind
const ofKind = (kind, fallback) => (country) => (kind.includes(local(country)) ? local(country) : fallback);
const DOLLAR = ["USD", "AUD", "CAD", "NZD", "SGD", "HKD", "TWD", "JMD", "TTD", "BSD", "BBD", "BZD", "FJD", "GYD", "LRD", "SRD", "BMD", "KYD", "XCD", "NAD", "SBD", "BND", "WST"];
const PESO = ["MXN", "PHP", "COP", "ARS", "CLP", "UYU", "DOP", "CUP"];
const RUPEE = ["INR", "PKR", "LKR", "NPR", "MUR", "SCR"];
const SIGNS = [
  ["US$", () => "USD"], ["A$", () => "AUD"], ["AU$", () => "AUD"], ["C$", () => "CAD"], ["CA$", () => "CAD"], ["NZ$", () => "NZD"],
  ["HK$", () => "HKD"], ["S$", () => "SGD"], ["NT$", () => "TWD"], ["J$", () => "JMD"], ["TT$", () => "TTD"], ["R$", () => "BRL"],
  ["Mex$", () => "MXN"], ["$", ofKind([...DOLLAR, ...PESO], "USD")], ["€", () => "EUR"], ["E£", () => "EGP"],
  ["£", ofKind(["EGP", "LBP", "SYP", "SDG", "GIP"], "GBP")], ["₹", () => "INR"], ["₦", () => "NGN"], ["₱", () => "PHP"], ["₩", () => "KRW"],
  ["¥", ofKind(["CNY"], "JPY")], ["₺", () => "TRY"], ["₽", () => "RUB"], ["₴", () => "UAH"], ["₫", () => "VND"], ["฿", () => "THB"],
  ["GH₵", () => "GHS"], ["₵", () => "GHS"], ["₪", () => "ILS"], ["৳", () => "BDT"], ["Rp", () => "IDR"], ["RM", () => "MYR"],
  ["Tk", () => "BDT"], ["KSh", () => "KES"], ["Ksh", () => "KES"], ["USh", () => "UGX"], ["TSh", () => "TZS"], ["Rs.", ofKind(RUPEE, "INR")],
  ["Rs", ofKind(RUPEE, "INR")], ["DH", ofKind(["MAD", "AED"], "MAD")], ["Dh", ofKind(["MAD", "AED"], "AED")], ["R", (c) => (["ZAR", "NAD", "LSL", "SZL"].includes(local(c)) ? "ZAR" : null)],
];
const WORDS = [
  ["US dollars", () => "USD"], ["euros", () => "EUR"], ["euro", () => "EUR"], ["dollars", ofKind(DOLLAR, "USD")], ["pesos", ofKind(PESO, null)],
  ["rupees", ofKind(RUPEE, "INR")], ["naira", () => "NGN"], ["cedis", () => "GHS"], ["shillings", ofKind(["KES", "UGX", "TZS", "SOS"], null)],
  ["rand", () => "ZAR"], ["ringgit", () => "MYR"], ["baht", () => "THB"], ["yuan", () => "CNY"], ["yen", () => "JPY"], ["taka", () => "BDT"],
  ["rupiah", () => "IDR"], ["dirhams", ofKind(["MAD", "AED"], null)], ["riyals", ofKind(["SAR", "QAR", "OMR", "IRR", "YER"], null)],
  ["rials", ofKind(["IRR", "OMR", "YER"], null)], ["dinars", ofKind(["DZD", "TND", "JOD", "KWD", "BHD", "IQD", "LYD", "RSD", "MKD"], null)],
  ["francs", ofKind(["CHF", "XOF", "XAF", "XPF", "BIF", "RWF", "GNF", "KMF", "DJF", "CDF"], null)], ["lira", ofKind(["TRY", "LBP"], "TRY")],
  ["kwacha", ofKind(["ZMW", "MWK"], null)], ["zlotys", () => "PLN"], ["zloty", () => "PLN"], ["forints", () => "HUF"], ["forint", () => "HUF"],
  ["rubles", () => "RUB"], ["roubles", () => "RUB"], ["hryvnias", () => "UAH"], ["tenge", () => "KZT"], ["kyat", () => "MMK"], ["dong", () => "VND"],
  ["kronor", () => "SEK"], ["kroner", ofKind(["NOK", "DKK"], null)], ["pounds sterling", () => "GBP"], ["Egyptian pounds", () => "EGP"],
];
// ISO codes written beside an amount ("USD 5,000", "5,000 EGP"); not those that are also English words
const CODES = new Set(Object.keys(rates.rates).filter((c) => !["ALL", "TOP", "CUP", "PEN", "MOP", "SOS", "BAM", "GEL", "TRY", "CAD"].includes(c)).concat("CAD"));
const SCALE = { thousand: 1e3, k: 1e3, lakh: 1e5, lakhs: 1e5, lac: 1e5, million: 1e6, mn: 1e6, m: 1e6, mln: 1e6, crore: 1e7, crores: 1e7, billion: 1e9, bn: 1e9, b: 1e9, trillion: 1e12 };

// weights: grams per unit, and whether the unit is metric (explained in pounds) or not (in grams)
const UNITS = [
  [["kilograms", "kilogrammes", "kilogram", "kilogramme", "kilos", "kilo", "kgs", "kg"], 1000, true, "kg"],
  [["grams", "grammes", "gram", "gramme", "gms", "gm", "g"], 1, true, "g"],
  [["milligrams", "milligram", "mg"], 0.001, true, "mg"],
  [["metric tonnes", "metric tons", "metric ton", "tonnes", "tonne", "t"], 1e6, true, "t"],
  [["pounds", "pound", "lbs", "lb"], 453.59237, false, "pound (lb)", "A pound (lb) is 453.6 grams."],
  [["ounces", "ounce", "oz"], 28.349523125, false, "ounce (oz)", "An ounce (oz) is 28.35 grams."],
  [["tons", "ton"], 907184.74, false, "ton", null],
  [["quintals", "quintal"], 1e5, false, "quintal", "A quintal is 100 kilograms."],
  [["tolas", "tola"], 11.6638, false, "tola", "A tola, a South Asian measure, is 11.66 grams."],
  [["maunds", "maund"], 37324.17, false, "maund", "A maund, a South Asian measure, is about 37.3 kilograms."],
  [["stones", "stone"], 6350.29318, false, "stone", "A stone is 14 pounds, 6.35 kilograms."],
  [["arrobas", "arroba"], 15000, false, "arroba", "An arroba, as used in Brazil, is 15 kilograms."],
];
// areas: square metres per unit; lengths: metres per unit. As above, with the kind of measure last.
// Lengths need the unit written out: a bare "m" is as often "million" ("5m plants").
const AREAS = [
  [["hectares", "hectare", "ha"], 1e4, true, "ha"],
  [["square metres", "square meters", "square metre", "square meter", "sq m", "sq. m", "m2", "m²"], 1, true, "m2"],
  [["square kilometres", "square kilometers", "square kilometre", "square kilometer", "sq km", "km2", "km²"], 1e6, true, "km2"],
  [["acres", "acre"], 4046.8564224, false, "acres", "An acre is about 4,047 square metres, or 0.405 hectares."],
  [["square feet", "square foot", "sq ft", "sq. ft", "ft2", "ft²"], 0.09290304, false, "sqft", "A square foot is about 0.093 square metres."],
  [["square miles", "square mile", "sq mi"], 2589988.110336, false, "sqmi", "A square mile is about 2.59 square kilometres, or 640 acres."],
  [["square yards", "square yard", "sq yd"], 0.83612736, false, "sqyd", "A square yard is about 0.836 square metres."],
  [["rai"], 1600, false, "rai", "A rai, a Thai land measure, is 1,600 square metres."],
  [["dunums", "dunum", "dunams", "dunam", "donums", "donum"], 1000, false, "dunum", "A dunum, used in the lands of the former Ottoman Empire, is about 1,000 square metres (2,500 in Iraq)."],
  [["mu"], 666.667, false, "mu", "A mu, a Chinese land measure, is about 667 square metres."],
  [["kanals", "kanal"], 505.857, false, "kanal", "A kanal, used in Pakistan and northern India, is about 506 square metres."],
  [["marlas", "marla"], 25.2929, false, "marla", "A marla, a twentieth of a kanal, is about 25 square metres."],
  [["feddans", "feddan"], 4200.83, false, "feddan", "A feddan, an Egyptian land measure, is about 4,200 square metres."],
  [["manzanas", "manzana"], 6987, false, "manzana", "A manzana, used in Central America, is about 0.7 hectares."],
  [["bighas", "bigha"], 1, false, "bigha"],
].map((u) => [...u.concat(Array(5 - u.length).fill(null)), "area"]);
const LENGTHS = [
  [["kilometres", "kilometers", "kilometre", "kilometer", "kms", "km"], 1000, true, "km"],
  [["metres", "meters", "metre", "meter"], 1, true, "m"],
  [["centimetres", "centimeters", "centimetre", "centimeter", "cm"], 0.01, true, "cm"],
  [["nautical miles", "nautical mile"], 1852, false, "nmi", "A nautical mile is 1,852 metres."],
  [["miles", "mile"], 1609.344, false, "miles", "A mile is about 1.61 kilometres."],
  [["feet", "foot", "ft"], 0.3048, false, "ft", "A foot is 0.3048 metres."],
  [["yards", "yard", "yd"], 0.9144, false, "yd", "A yard is 0.9144 metres."],
  [["inches", "inch"], 0.0254, false, "in", "An inch is 2.54 centimetres."],
].map((u) => [...u.concat(Array(5 - u.length).fill(null)), "length"]);
const unitOf = new Map([...UNITS, ...AREAS, ...LENGTHS].flatMap((u) => u[0].map((word) => [word.toLowerCase(), u])));
// how a unit may be written: as listed; a whole word also capitalised ("Kilograms"), and kg and lb
// also in capitals ("KG", "LBS"); never "G" or "M", which are networks, sizes and grades
const unitForms = [...unitOf.keys()].flatMap((w) => [w, ...(w.length > 3 ? [w[0].toUpperCase() + w.slice(1)] : []), ...(["kg", "kgs", "lb", "lbs"].includes(w) ? [w.toUpperCase()] : [])]);

const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const NUM = String.raw`\d{1,3}(?:,\d{2,3})+(?:\.\d+)?|\d+(?:\.\d+)?`;
// a scale after the number: a word ("1.5 crore", "2-million"), or k, m, bn written straight after it ("$5m")
const SCALES = String.raw`(?:[\s-]?(thousand|lakhs?|lac|million|mln|crores?|billion|trillion|mn|bn)\b|(k|m|b)\b)?`;
const byLength = (list) => [...list].sort((a, b) => b.length - a.length).map(esc).join("|");
const signs = byLength(SIGNS.map(([s]) => s));
const words = byLength(WORDS.map(([w]) => w));
const codes = byLength([...CODES]);
const units = byLength(unitForms);
const NB = String.raw`(?<![\p{L}\p{N}$£€¥₹])`;
const MONEY_BEFORE = new RegExp(String.raw`${NB}(${signs}|${codes})\s?(${NUM})${SCALES}`, "gu");
// a currency word may follow a nationality: "11,800 Turkish lira", "500 Australian dollars"
const MONEY_AFTER = new RegExp(String.raw`${NB}(${NUM})${SCALES}[\s-]+(?:([A-Z][a-z]+)\s)?(${words}|${codes})(?![\p{L}])`, "gu");
const WEIGHT = new RegExp(String.raw`${NB}(${NUM})${SCALES}\s?-?\s?(${units})(?![\p{L}\p{N}])`, "gu");
// "a 10,000-pound fine", "fined 500 pounds": pounds of money, not of weight
const POUND_MONEY = /\b(fine[ds]?|bail|penalty|worth|valued?|cost|price|paid|pay|salary|reward|bribe|sterling)\b/i;

const number = (n, scale = "") => Number(n.replace(/,/g, "")) * (SCALE[(scale ?? "").toLowerCase()] ?? 1);
const sig = (v, digits = 3) => v.toLocaleString("en-GB", { maximumSignificantDigits: digits });
const big = (v) => (v >= 1e9 ? `${sig(v / 1e9)} billion` : v >= 1e6 ? `${sig(v / 1e6)} million` : sig(v));
const money = (v, code, digits = 3) => {
  if (v >= 1e6) return `${new Intl.NumberFormat("en-GB", { style: "currency", currency: code, maximumSignificantDigits: digits }).format(v >= 1e9 ? v / 1e9 : v / 1e6)} ${v >= 1e9 ? "billion" : "million"}`;
  return new Intl.NumberFormat("en-GB", { style: "currency", currency: code, maximumSignificantDigits: digits }).format(v);
};
const exact = (v, code) => new Intl.NumberFormat("en-GB", { style: "currency", currency: code, minimumFractionDigits: 0, maximumFractionDigits: 2 }).format(v);
let names, byName;
const currencyName = (code) => { try { names ??= new Intl.DisplayNames("en", { type: "currency" }); return names.of(code); } catch { return code; } };
// "Turkish lira" → TRY: a currency's English name, in the singular, without "s" endings
const nameToCode = (phrase) => {
  byName ??= new Map(Object.keys(rates.rates).map((c) => [currencyName(c).toLowerCase(), c]));
  const p = phrase.toLowerCase().replace(/(?<=[a-z])s$/, "");
  return byName.get(p) ?? byName.get(p.replace(/ie$/, "y")) ?? null;
};
const RATES_DATE = new Date(`${rates.date}T00:00:00Z`).toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
const NOTE_SCALE = { lakh: "A lakh is 100,000.", lakhs: "A lakh is 100,000.", lac: "A lakh is 100,000.", crore: "A crore is 10 million.", crores: "A crore is 10 million." };

// An entry for the glossary's tooltip: its rows (unit, value) shown as a small table, a note under
// it, and the same in one sentence (text) for screen readers and as a fallback.
const entry = (kind, written, eyebrow, rows, note) => ({
  id: `${kind}:${written}`, title: written, eyebrow, rows, note,
  text: `${rows.map(([label, value]) => `${label}: ${value}`).join("; ")}. ${note}`.replace(/\s+/g, " ").trim(),
});
const RATES_NOTE = `At the exchange rates of ${RATES_DATE} (${rates.source}), not those at the time of the event.`;
function moneyEntry(written, code, amount, scale) {
  const usd = amount / rates.rates[code];
  const rows = [[currencyName(code), exact(amount, code)]];
  for (const [to, name] of [["USD", "US dollar"], ["EUR", "Euro"], ["GBP", "British pound"]]) if (to !== code) rows.push([name, `≈ ${money(usd * rates.rates[to], to)}`]);
  return entry("money", written, "Converted", rows, `${NOTE_SCALE[(scale ?? "").toLowerCase()] ?? ""} ${RATES_NOTE}`.trim());
}
// a weight in the units that suit its size, leaving out the one it is written in
function weightRows(grams, written) {
  const lb = grams / 453.59237, oz = grams / 28.349523125, rows = [];
  if (grams >= 1e6) rows.push(["Tonnes", `${big(grams / 1e6)} t`, "t"]);
  if (grams >= 1000) rows.push(["Kilograms", `${big(grams / 1000)} kg`, "kg"]);
  else rows.push(["Grams", `${sig(grams)} g`, "g"]);
  if (lb >= 0.1) rows.push(["Pounds", `${big(lb)} lb`, "lb"]);
  if (lb < 100) rows.push(["Ounces", `${sig(oz)} oz`, "oz"]);
  if (grams >= 1e6) rows.push(["US tons", `${big(grams / 907184.74)} US tons`, "ton"]);
  return rows.filter((r) => r[2] !== written).map(([label, value]) => [label, `≈ ${value}`]);
}
const FAMILY = { kg: "kg", g: "g", mg: "g", t: "t", "pound (lb)": "lb", "ounce (oz)": "oz", ton: "ton" };
// an area in the units that suit its size, leaving out the one it is written in
function areaRows(m2, written) {
  const rows = [];
  if (m2 >= 1e6) rows.push(["Square kilometres", `${big(m2 / 1e6)} km²`, "km2"]);
  if (m2 >= 1000) rows.push(["Hectares", `${big(m2 / 1e4)} ha`, "ha"]);
  if (m2 < 1e6) rows.push(["Square metres", `${big(m2)} m²`, "m2"]);
  if (m2 >= 400) rows.push(["Acres", `${big(m2 / 4046.8564224)} acres`, "acres"]);
  if (m2 < 2000) rows.push(["Square feet", `${big(m2 / 0.09290304)} sq ft`, "sqft"]);
  if (m2 >= 2.59e6) rows.push(["Square miles", `${big(m2 / 2589988.110336)} sq miles`, "sqmi"]);
  return rows.filter((r) => r[2] !== written).map(([label, value]) => [label, `≈ ${value}`]);
}
function lengthRows(m, written) {
  const rows = [];
  if (m >= 1000) rows.push(["Kilometres", `${big(m / 1000)} km`, "km"]);
  if (m < 1000) rows.push(["Metres", `${big(m)} m`, "m"]);
  if (m >= 500) rows.push(["Miles", `${big(m / 1609.344)} miles`, "miles"]);
  if (m < 3000) rows.push(["Feet", `${big(m / 0.3048)} ft`, "ft"]);
  return rows.filter((r) => r[2] !== written).map(([label, value]) => [label, `≈ ${value}`]);
}
// A bigha's size depends on the region (local land records give the exact figure)
const BIGHAS = { in: [["Himachal Pradesh (about 809 m²)", 809.4], ["West Bengal and Assam (about 1,338 m²)", 1337.8], ["Rajasthan, Uttar Pradesh, Bihar (about 2,529 m²)", 2529.3]], np: [["Nepal (about 6,773 m²)", 6772.63]] };
function areaEntry(written, amount, unit, country) {
  const [, , metric, name, about] = unit;
  if (name === "bigha") {
    const sizes = BIGHAS[String(country ?? "").split("/")[0]] ?? [...BIGHAS.in, ...BIGHAS.np];
    const rows = sizes.map(([where, m2]) => [where, `≈ ${big((amount * m2) / 1e4)} ha · ${big((amount * m2) / 4046.8564224)} acres`]);
    return entry("area", written, "In other units", rows, "A bigha's size depends on the region; local land records give the exact figure.");
  }
  const rows = areaRows(amount * unit[1], name);
  const defines = [rows.some(([l]) => l === "Acres") && "An acre is about 4,047 square metres.", rows.some(([l]) => l === "Square feet") && "A square foot is about 0.093 square metres.", rows.some(([l]) => l === "Square miles") && "A square mile is about 2.59 square kilometres."];
  return entry("area", written, "In other units", rows, metric ? defines.filter(Boolean).join(" ") : about);
}
function lengthEntry(written, amount, unit) {
  const [, , metric, name, about] = unit;
  const rows = lengthRows(amount * unit[1], name);
  const defines = [rows.some(([l]) => l === "Miles") && "A mile is about 1.61 kilometres.", rows.some(([l]) => l === "Feet") && "A foot is 0.3048 metres."];
  return entry("length", written, "In other units", rows, metric ? defines.filter(Boolean).join(" ") : about);
}
function weightEntry(written, grams, unit, country) {
  const [, , metric, name, about] = unit;
  if (name === "ton") {
    // a US "short" ton of 2,000 pounds; elsewhere a ton usually means a metric tonne of 1,000 kg
    const count = grams / 907184.74, us = local(country) === "USD";
    const asUS = ["If US tons (2,000 lb)", `≈ ${big(grams / 1e6)} t · ${big(grams / 453.59237)} lb`];
    const asMetric = ["If metric tonnes", `≈ ${big(count)} t · ${big((count * 1e6) / 453.59237)} lb`];
    return entry("weight", written, "In other units", us ? [asUS, asMetric] : [asMetric, asUS],
      us ? "In the US a ton is 2,000 pounds (907 kg)." : "Outside the US a ton usually means a metric tonne of 1,000 kg; a US ton is 2,000 pounds (907 kg).");
  }
  const rows = weightRows(grams, FAMILY[name]);
  const defines = [rows.some(([l]) => l === "Pounds") && "A pound (lb) is 453.6 grams.", rows.some(([l]) => l === "Ounces") && "An ounce (oz) is 28.35 grams.", rows.some(([l]) => l === "US tons") && "A US ton is 2,000 pounds."];
  return entry("weight", written, "In other units", rows, metric ? defines.filter(Boolean).join(" ") : about);
}

// Every weight and sum of money in a text: [{ at, text, entry }] in order, never overlapping.
export function measureMatches(text, { country } = {}) {
  const s = String(text ?? ""), found = [];
  const add = (at, written, entry) => { if (entry && !found.some((f) => at < f.at + f.text.length && f.at < at + written.length)) found.push({ at, text: written, entry }); };
  for (const m of s.matchAll(MONEY_BEFORE)) {
    const [written, sign, n, scaleWord, scaleShort] = m, scale = scaleWord ?? scaleShort;
    const code = CODES.has(sign) && /^[A-Z]{3}$/.test(sign) ? sign : SIGNS.find(([x]) => x === sign)?.[1](country);
    if (code && rates.rates[code]) add(m.index, written, moneyEntry(written, code, number(n, scale), scale));
  }
  for (const m of s.matchAll(MONEY_AFTER)) {
    const [, n, scaleWord, scaleShort, nation, word] = m, scale = scaleWord ?? scaleShort;
    // "Turkish lira", "Australian dollars": the currency of that name, else the word read alone
    const named = nation && nameToCode(`${nation} ${word}`);
    const code = named || (/^[A-Z]{3}$/.test(word) && CODES.has(word) ? word : WORDS.find(([w]) => w.toLowerCase() === word.toLowerCase())?.[1](country));
    const written = named ? m[0] : m[0].replace(new RegExp(String.raw`${nation} (?=${esc(word)}$)`), "");
    if (code && rates.rates[code]) add(m.index, written, moneyEntry(written, code, number(n, scale), scale));
  }
  for (const m of s.matchAll(WEIGHT)) {
    const [written, n, scaleWord, scaleShort, word] = m, scale = scaleWord ?? scaleShort;
    const unit = unitOf.get(word.toLowerCase());
    if (!unit) continue;
    if (unit[3]?.startsWith("pound") && POUND_MONEY.test(s.slice(Math.max(0, m.index - 40), m.index + written.length + 16))) {
      const code = local(country) === "EGP" ? "EGP" : "GBP";
      add(m.index, written, moneyEntry(written, code, number(n, scale), scale));
      continue;
    }
    const amount = number(n, scale);
    add(m.index, written, unit[5] === "area" ? areaEntry(written, amount, unit, country)
      : unit[5] === "length" ? lengthEntry(written, amount, unit)
      : weightEntry(written, amount * unit[1], unit, country));
  }
  return found.sort((a, b) => a.at - b.at);
}
