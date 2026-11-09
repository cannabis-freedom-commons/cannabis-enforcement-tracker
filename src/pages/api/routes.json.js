// The smuggling routes the home page's Routes view draws (src/scripts/routes-view.js), loaded only
// when that view opens: one short record per report with a route, and the places they name
// (src/lib/routes.js). Not part of the public API; the routes are in /api/reports.json.
import { reports } from "../../lib/reports.js";
import { centreOf } from "../../lib/map.js";
import { countryName, flagOf } from "../../lib/countries.js";
import { routeRecords, routeNodes } from "../../lib/routes.js";

export function GET() {
  const records = routeRecords(reports);
  const nodes = routeNodes(records, { centreOf, countryName, flagOf });
  return new Response(JSON.stringify({ v: 1, records, nodes }), { headers: { "Content-Type": "application/json" } });
}
