// Smuggling routes for the Routes page: the reports that name where the cannabis came from and where
// it was going (details.route), the corridors they add up to, and the curved lines the map draws.
// Plain functions of plain data, shared by the page (at build time) and its script (in the browser).

// How the cannabis travelled, from the places a report names (details.setting): the page's filter.
export const MODES = [
  ["air", "By air", ["airport"]],
  ["road", "By road or across a border", ["vehicle", "border"]],
  ["sea", "By sea", ["port"]],
  ["mail", "By post", ["mail"]],
];
// The kinds of cannabis the dots are coloured by.
export const PRODUCTS = [
  ["hashish", "Hashish", "#d39a45"],
  ["flower", "Flower", "#73c27a"],
  ["other", "Oil, edibles and other", "#b996e6"],
  ["", "Not stated", "#cfc9b8"],
];
export const productOf = (list = []) => (list.includes("hashish") ? "hashish" : list.includes("flower") || list.includes("plants") ? "flower" : list.length ? "other" : "");
export const modesOf = (settings = []) => MODES.filter(([, , s]) => s.some((x) => settings.includes(x))).map(([id]) => id);

// One record per report with a route, with short keys like the report index: id, url, title, date,
// year, from, to, crossing, kg seized, products, modes.
export function routeRecords(reports) {
  return reports
    .filter((r) => r.details?.route?.from?.length && r.details.route.to?.length)
    .map((r) => {
      const d = r.details, products = d.products?.map((p) => p.product) ?? [];
      return {
        i: r.id, u: r.url, ti: r.title, d: r.date.toISOString().slice(0, 10), y: r.date.getUTCFullYear(),
        f: [...new Set(d.route.from)], t: [...new Set(d.route.to)],
        ...(d.route.crossing ? { x: d.route.crossing } : {}),
        ...(r.figures?.kg_seized ? { kg: r.figures.kg_seized } : {}),
        p: productOf(products), m: modesOf(d.setting),
      };
    })
    .sort((a, b) => b.d.localeCompare(a.d));
}

// The places the records name, keyed by id: name, flag, and where the line ends ([lon, lat], the
// middle of the country; a state goes to its country's middle).
export function routeNodes(records, { centreOf, countryName, flagOf }) {
  const ids = new Set(records.flatMap((r) => [...r.f, ...r.t]));
  const out = {};
  for (const id of [...ids].sort()) {
    const c = centreOf(id);
    if (c) out[id] = { n: countryName(id), fl: flagOf(id), ll: [c[1], c[0]] };
  }
  return out;
}

// A route between two places drawn at the same point (the same country, or a state inside it) is
// domestic: it is counted, not drawn.
const samePlace = (nodes, a, b) => a === b || (nodes[a] && nodes[b] && nodes[a].ll[0] === nodes[b].ll[0] && nodes[a].ll[1] === nodes[b].ll[1]);

// The corridors the records add up to: every from × to pair of each record, so a report naming two
// destinations counts on both lines (its kilograms too). Domestic routes are returned apart.
export function corridors(records, nodes) {
  const lines = new Map(), domestic = new Map();
  for (const r of records) for (const from of r.f) for (const to of r.t) {
    if (!nodes[from] || !nodes[to]) continue;
    const local = samePlace(nodes, from, to);
    const key = local ? from.split("/")[0] : `${from}>${to}`;
    const into = local ? domestic : lines;
    const c = into.get(key) ?? { key, from: local ? key : from, to: local ? key : to, reports: 0, kg: 0, first: r.y, last: r.y, ids: [], products: {}, modes: {} };
    c.reports++; c.kg += r.kg ?? 0;
    c.first = Math.min(c.first, r.y); c.last = Math.max(c.last, r.y);
    c.ids.push(r.i);
    c.products[r.p] = (c.products[r.p] ?? 0) + 1;
    for (const m of r.m) c.modes[m] = (c.modes[m] ?? 0) + 1;
    into.set(key, c);
  }
  const order = (a, b) => b.reports - a.reports || b.kg - a.kg || a.key.localeCompare(b.key);
  return { lines: [...lines.values()].sort(order), domestic: [...domestic.values()].sort(order) };
}

// The places on the lines: how many reports leave each and arrive in it, so the map can say which
// mostly send and which mostly receive.
export function placeTotals(lines) {
  const out = {};
  for (const c of lines) {
    (out[c.from] ??= { out: 0, in: 0 }).out += c.reports;
    (out[c.to] ??= { out: 0, in: 0 }).in += c.reports;
  }
  return out;
}

// The line on the map, flat or globe: a gentle curve from a to b ([lon, lat]), bending to the left
// of the direction of travel, so the two directions between the same places are two separate lines.
// The shorter way round is taken (the end may lie past ±180°, which the map draws fine). The map
// cannot draw beyond about 85° north or south and would squash a line there into a ring round the
// pole, so a curve that would rise above 80° bends less, and as a last resort is held at 80°.
export const MAX_LAT = 80;
export function arc(a, b, steps = 48) {
  let [x1, y1] = a, [x2, y2] = b;
  if (x2 - x1 > 180) x2 -= 360;
  else if (x1 - x2 > 180) x2 += 360;
  const dx = x2 - x1, dy = y2 - y1, len = Math.hypot(dx, dy);
  let bend = Math.min(0.22, 0.12 + len / 900); // longer lines bend a little more
  let out = [];
  for (let tries = 0; tries < 8; tries++, bend *= 0.6) {
    const cx = (x1 + x2) / 2 - dy * bend, cy = (y1 + y2) / 2 + dx * bend;
    out = [];
    for (let i = 0; i <= steps; i++) {
      const t = i / steps, u = 1 - t;
      out.push([+(u * u * x1 + 2 * u * t * cx + t * t * x2).toFixed(4), +(u * u * y1 + 2 * u * t * cy + t * t * y2).toFixed(4)]);
    }
    if (out.every(([, y]) => Math.abs(y) <= MAX_LAT)) return out;
  }
  return out.map(([x, y]) => [x, Math.max(-MAX_LAT, Math.min(MAX_LAT, y))]);
}

// whether a point ([lon, lat]) faces the viewer of a globe centred on `centre`: on the near side
export function nearSide(point, centre) {
  const rad = Math.PI / 180;
  const [l1, p1] = [point[0] * rad, point[1] * rad], [l2, p2] = [centre[0] * rad, centre[1] * rad];
  return Math.sin(p1) * Math.sin(p2) + Math.cos(p1) * Math.cos(p2) * Math.cos(l1 - l2) > 0.05;
}

// a point part of the way (0–1) along a line of points
export function along(line, t) {
  const pos = Math.max(0, Math.min(1, t)) * (line.length - 1), i = Math.min(line.length - 2, Math.floor(pos)), f = pos - i;
  return [line[i][0] + (line[i + 1][0] - line[i][0]) * f, line[i][1] + (line[i + 1][1] - line[i][1]) * f];
}
