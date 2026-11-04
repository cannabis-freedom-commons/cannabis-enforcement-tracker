// Aggregate at build time; the statistics page receives counts, never report bodies.
export function sourcesByDomain(reports) {
  const domains = new Map();
  for (const report of reports) {
    const cited = new Set();
    for (const source of report.sources ?? []) {
      let url;
      try { url = new URL(source.url); } catch { continue; }
      if (!["https:", "http:"].includes(url.protocol)) continue;
      const domain = url.hostname.replace(/^www\./, "");
      url.hostname = domain;
      url.hash = "";
      let entry = domains.get(domain);
      if (!entry) domains.set(domain, entry = { domain, urls: new Set(), reports: 0 });
      entry.urls.add(url.href);
      cited.add(domain);
    }
    for (const domain of cited) domains.get(domain).reports++;
  }
  return [...domains.values()]
    .map(({ domain, urls, reports }) => ({ domain, sources: urls.size, reports }))
    .sort((a, b) => b.sources - a.sources || b.reports - a.reports || a.domain.localeCompare(b.domain));
}

// The kind of site a source is on, by its address alone: official (government, police, courts),
// social media, a web archive, or news and other sites. A heuristic: it reads only the host name.
const OFFICIAL_LABELS = new Set(["gov", "gob", "gouv", "govt", "go", "mil", "gc", "gub", "gv", "police", "polizei", "policia", "politie", "court", "courts", "judiciary", "justice"]);
const OFFICIAL_HOSTS = ["garda.ie", "elitigation.sg", "mvdmedia.ru", "police.uk", "europa.eu", "unodc.org", "un.org", "admin.ch", "bund.de"];
const SOCIAL_HOSTS = ["facebook.com", "fb.watch", "x.com", "twitter.com", "instagram.com", "youtube.com", "youtu.be", "tiktok.com", "t.me", "vk.com", "ok.ru", "threads.net", "linkedin.com", "reddit.com"];
const ARCHIVE_HOSTS = ["web.archive.org", "archive.org", "archive.ph", "archive.today", "archive.is", "archive.md"];
const under = (host, list) => list.some((h) => host === h || host.endsWith(`.${h}`));
export const SOURCE_KINDS = [["official", "Official", "Government, police and court sites"], ["news", "News and other", "News outlets, advocacy groups and other sites"], ["social", "Social media", "Posts on social networks"], ["archive", "Web archives", "Archived copies"]];
export function sourceKind(host) {
  const h = String(host).toLowerCase().replace(/^www\./, "");
  if (under(h, ARCHIVE_HOSTS)) return "archive";
  if (under(h, SOCIAL_HOSTS)) return "social";
  if (under(h, OFFICIAL_HOSTS) || h.split(".").slice(0, -1).some((l) => OFFICIAL_LABELS.has(l)) || /^(gov|mil)$/.test(h.split(".").at(-1))) return "official";
  return "news";
}

// country-code domains that are mostly sold as generic names, not used by the country
export const GENERIC_CC = new Set(["tv", "io", "me", "fm", "ai", "cc", "ws", "co", "ly", "to", "gg", "am"]);

// distinct source addresses by top-level domain (".com", ".br"), and by kind of site, most first:
// { tlds: [{ tld, sources, reports }], kinds: { official: n, … }, total }
export function sourcesByTld(reports) {
  const tlds = new Map(), kinds = Object.fromEntries(SOURCE_KINDS.map(([k]) => [k, 0])), seen = new Set();
  for (const report of reports) {
    const cited = new Set();
    for (const source of report.sources ?? []) {
      let url;
      try { url = new URL(source.url); } catch { continue; }
      if (!["https:", "http:"].includes(url.protocol)) continue;
      const host = url.hostname.toLowerCase().replace(/^www\./, "").replace(/\.$/, "");
      const tld = host.split(".").at(-1);
      if (!/^[a-z0-9-]+$/.test(tld) || /^\d+$/.test(tld)) continue;
      url.hostname = host; url.hash = "";
      cited.add(tld);
      const entry = tlds.get(tld) ?? tlds.set(tld, { tld, urls: new Set(), reports: 0 }).get(tld);
      entry.urls.add(url.href);
      if (!seen.has(url.href)) { seen.add(url.href); kinds[sourceKind(host)]++; }
    }
    for (const tld of cited) tlds.get(tld).reports++;
  }
  return {
    tlds: [...tlds.values()].map(({ tld, urls, reports }) => ({ tld, sources: urls.size, reports }))
      .sort((a, b) => b.sources - a.sources || b.reports - a.reports || a.tld.localeCompare(b.tld)),
    kinds, total: seen.size,
  };
}
