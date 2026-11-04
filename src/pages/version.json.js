// The deployed build, polled by open pages (src/scripts/site-updates.js) to offer a refresh when a
// newer one is out. Kept tiny: it is fetched every minute while a page is open.
import { build } from "../lib/build-info.js";

export function GET() {
  return new Response(JSON.stringify(build), { headers: { "Content-Type": "application/json" } });
}
