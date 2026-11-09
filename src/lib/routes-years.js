// The years the routes cover and how many reports name a route in each, by what was carried: the
// Routes view's timeline (RoutesBar) and year range. Build time only.
import { reports } from "./reports.js";
import { routeRecords, PRODUCTS } from "./routes.js";

let cached = null;
export function routeYears() {
  if (cached) return cached;
  const records = routeRecords(reports);
  const first = Math.min(...records.map((r) => r.y)), last = Math.max(...records.map((r) => r.y));
  const perYear = Array.from({ length: last - first + 1 }, (_, i) => {
    const list = records.filter((r) => r.y === first + i);
    return { year: first + i, total: list.length, products: PRODUCTS.map(([id]) => list.filter((r) => r.p === id).length) };
  });
  return (cached = { first, last, perYear });
}
