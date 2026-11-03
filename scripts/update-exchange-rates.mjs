// The exchange rates the site's money tooltips convert with (src/lib/measures.js): one day's rates
// against the US dollar from ExchangeRate-API's free open feed, kept in the repository so builds
// stay offline and every tooltip can name the date. Run now and then to refresh.
//
//   node scripts/update-exchange-rates.mjs
import fs from "node:fs";

const URL = "https://open.er-api.com/v6/latest/USD";
const response = await fetch(URL, { signal: AbortSignal.timeout(20000) });
if (!response.ok) throw new Error(`${URL}: HTTP ${response.status}`);
const data = await response.json();
if (data.result !== "success" || !data.rates?.EUR) throw new Error(`${URL}: no rates in the answer`);
const out = {
  source: "ExchangeRate-API",
  url: "https://www.exchangerate-api.com",
  date: new Date(data.time_last_update_unix * 1000).toISOString().slice(0, 10),
  base: "USD",
  rates: Object.fromEntries(Object.entries(data.rates).sort(([a], [b]) => a.localeCompare(b))),
};
fs.writeFileSync("src/data/exchange-rates.json", `${JSON.stringify(out, null, 2)}\n`);
console.log(`${Object.keys(out.rates).length} rates of ${out.date} written to src/data/exchange-rates.json`);
