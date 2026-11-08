// Invoked by the Go source loader. The browser's only way out is the loader's proxy, which lets it
// reach public hosts only (port 443, and 80 for plain HTTP); Chromium makes its own connections
// through it, so a site sees a browser's TLS, HTTP/2 and headers. No account, saved cookies or
// provider keys load.
import { chromium } from "playwright";
import { PlaywrightBlocker, fromPlaywrightDetails } from "@ghostery/adblocker-playwright";
import fs from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { saveCaptureImage } from "./archive-screenshots.mjs";

// Ads, trackers and cookie notices are kept out of the capture, so the screenshot and the text show
// the article: Ghostery's filter lists (EasyList, EasyPrivacy and the annoyance lists, cookie notices
// included) block their requests and hide what is left, and DuckDuckGo's Autoconsent answers the
// consent dialogs it knows by refusing optional cookies, which also lifts the overlays that stop a
// page from scrolling. The filter engine is built once and cached for a few days.
const require = createRequire(import.meta.url);
const BLOCKER_CACHE = process.env.SOURCE_BLOCKER_CACHE || path.join(path.dirname(fileURLToPath(import.meta.url)), "..", ".cache", "source-blocker.bin");
const BLOCKER_MAX_AGE = 4 * 24 * 3600 * 1000;
let blockerPromise;
export function loadBlocker() {
  return blockerPromise ??= (async () => {
    if (process.env.SOURCE_ADBLOCK === "0") return null; // switched off
    try {
      const stat = await fs.stat(BLOCKER_CACHE).catch(() => null);
      if (stat && Date.now() - stat.mtimeMs > BLOCKER_MAX_AGE) await fs.rm(BLOCKER_CACHE, { force: true });
      await fs.mkdir(path.dirname(BLOCKER_CACHE), { recursive: true });
      return await PlaywrightBlocker.fromPrebuiltFull(fetch, { path: BLOCKER_CACHE, read: fs.readFile, write: fs.writeFile });
    } catch { return null; } // no lists (offline): capture without them
  })();
}
let autoconsent;
async function loadAutoconsent() {
  if (autoconsent !== undefined) return autoconsent;
  try {
    // the package exports its rules but not its Playwright script, which sits beside them
    const rulesPath = require.resolve("@duckduckgo/autoconsent/rules/rules.json");
    const script = await fs.readFile(path.join(path.dirname(rulesPath), "..", "dist", "autoconsent.playwright.js"), "utf8");
    const { autoconsent: rules } = JSON.parse(await fs.readFile(rulesPath, "utf8"));
    autoconsent = { script, rules };
  } catch { autoconsent = null; }
  return autoconsent;
}

// Autoconsent's content script talks to us through a binding: it asks for its rules, may ask us to
// run a snippet in the page, and reports what it found and did.
async function enableAutoconsent(page) {
  const ac = await loadAutoconsent();
  const state = { detected: false, done: false, cmp: "" };
  if (!ac) return state;
  const send = (frame, message) => frame.evaluate((m) => window.autoconsentReceiveMessage?.(m), message).catch(() => {});
  await page.exposeBinding("autoconsentSendMessage", async ({ frame }, message) => {
    if (!message || typeof message !== "object") return;
    if (process.env.SOURCE_BLOCKER_DEBUG && message.type !== "eval") console.error("autoconsent", message.type, message.cmp || "", message.result ?? "");
    switch (message.type) {
      case "init":
        await send(frame, { type: "initResp", rules: { autoconsent: ac.rules }, config: { enabled: true, autoAction: "optOut", disabledCmps: [], enablePrehide: true, enableCosmeticRules: true, detectRetries: 20, isMainWorld: true } });
        break;
      case "eval": {
        let result = false;
        try { result = await frame.evaluate(String(message.code)); } catch { /* the frame navigated or the snippet failed */ }
        await send(frame, { id: message.id, type: "evalResp", result });
        break;
      }
      case "cmpDetected": case "popupFound": state.detected = true; state.cmp = String(message.cmp || state.cmp); break;
      case "optOutResult": case "autoconsentDone": state.done = true; break;
    }
  });
  await page.addInitScript({ content: ac.script });
  return state;
}

export function extractSourcePage() {
  const clean = (value, max = 2000) => String(value || "").replace(/\s+/g, " ").trim().slice(0, max);
  const tags = {};
  for (const element of [...document.querySelectorAll("meta[name], meta[property]")].slice(0, 160)) {
    const key = (element.getAttribute("property") || element.getAttribute("name") || "").toLowerCase();
    if (key && key.length < 100 && !/(?:^|[-_:])(token|secret|password|authorization|csrf)(?:$|[-_:])|api[_-]?key/i.test(key) && !Object.hasOwn(tags, key)) tags[key] = clean(element.getAttribute("content"));
  }
  const jsonLD = [];
  for (const element of [...document.querySelectorAll('script[type="application/ld+json"]')].slice(0, 8)) {
    if (element.textContent.length > 32000) continue;
    try { jsonLD.push(JSON.parse(element.textContent)); } catch { /* preserve only valid structured data */ }
  }
  let article = {};
  const visit = (value, depth = 0) => {
    if (!value || typeof value !== "object" || depth > 6) return;
    if (Array.isArray(value)) { value.slice(0, 50).forEach(item => visit(item, depth + 1)); return; }
    const types = [].concat(value["@type"] || []);
    if (types.some(type => /^(NewsArticle|Article|BlogPosting|ReportageNewsArticle)$/.test(type)) && !article["@type"]) article = value;
    if (value["@graph"]) visit(value["@graph"], depth + 1);
  };
  jsonLD.forEach(value => visit(value));
  const authors = [].concat(article.author || []).map(author => clean(typeof author === "string" ? author : author?.name, 200)).filter(Boolean);
  if (tags.author && !authors.includes(tags.author)) authors.unshift(tags.author);
  let canonical = document.querySelector('link[rel~="canonical"]')?.getAttribute("href") || "";
  try { canonical = new URL(canonical, document.baseURI).href; } catch { canonical = ""; }
  if (!document.querySelector('link[rel~="canonical"]')) canonical = "";
  const metadata = {
    meta: tags, keywords: clean(tags.keywords),
    title: clean(document.title || tags["og:title"] || article.headline, 500),
    description: clean(tags.description || tags["og:description"] || tags["twitter:description"] || article.description),
    canonical, language: clean(document.documentElement.lang || tags["og:locale"] || article.inLanguage, 40),
    authors: authors.slice(0, 20), published_at: clean(tags["article:published_time"] || tags.date || tags.pubdate || tags.datepublished || tags["dcterms.created"] || article.datePublished, 100),
    modified_at: clean(tags["article:modified_time"] || article.dateModified, 100),
    site_name: clean(tags["og:site_name"] || tags["application-name"] || article.publisher?.name, 200), robots: clean(tags.robots, 500),
    open_graph: Object.fromEntries(Object.entries(tags).filter(([key]) => key.startsWith("og:") || key.startsWith("article:"))),
    twitter: Object.fromEntries(Object.entries(tags).filter(([key]) => key.startsWith("twitter:"))), json_ld: jsonLD,
  };
  // Related-story cards often use <article> too. Anchor the capture to the visible
  // headline and main content before considering standalone article elements.
  const unrelated = "nav, aside, footer, [role=banner], [role=navigation], [role=complementary], [class*=related], [class*=recommend], [class*=promo], [class*=advert]";
  const visible = element => element && element.getClientRects().length && !element.closest(unrelated);
  const title = clean(tags["og:title"] || document.title).toLocaleLowerCase();
  const headlines = [...document.querySelectorAll("h1")].filter(visible);
  const headlineScore = element => {
    const text = clean(element.innerText).toLocaleLowerCase();
    return text && title.includes(text) ? text.length : 0;
  };
  headlines.sort((a, b) => headlineScore(b) - headlineScore(a));
  const headline = headlines[0];
  const byLength = (a, b) => b.innerText.trim().length - a.innerText.trim().length;
  const mains = [...document.querySelectorAll("main, [role=main]")].filter(visible).sort(byLength);
  const articles = [...document.querySelectorAll("article")].filter(visible).sort(byLength);
  const candidates = [headline?.closest("article"), headline?.closest("main, [role=main]"), ...mains,
    // A headline outside semantic containers is common on custom-element sites.
    // In that case, the body is safer than an unrelated article card.
    ...(headline ? [document.body] : articles), document.body];
  const content = candidates.find(element => (element?.innerText || "").trim().length >= 200) || document.body;
  let text = (content?.innerText || "").trim();
  // Web-component sites (MSN, for example) render the article inside open shadow roots, which
  // innerText and the HTML clone never see. When the light DOM has too little text or no headline
  // matching the title (only a cookie banner, say), read the rendered composed tree instead,
  // anchored to the visible headline: MSN stacks the next stories of an endless feed under the
  // article, so the whole body would mix in other events.
  if (text.length < 200 || !(headline && headlineScore(headline))) {
    const block = /^(address|article|aside|blockquote|body|br|dd|div|dl|dt|figcaption|figure|footer|h[1-6]|header|hr|li|main|ol|p|pre|section|table|tr|ul)$/;
    const composedText = root => {
      const parts = [];
      const walk = node => {
        if (node.nodeType === Node.TEXT_NODE) { parts.push(node.data); return; }
        if (node.nodeType === Node.DOCUMENT_FRAGMENT_NODE) { node.childNodes.forEach(walk); return; }
        if (node.nodeType !== Node.ELEMENT_NODE || /^(script|style|noscript|template|svg)$/.test(node.localName)) return;
        if (node.checkVisibility && !node.checkVisibility({ visibilityProperty: true })) return;
        if (node.localName === "slot") { const assigned = node.assignedNodes({ flatten: true }); (assigned.length ? assigned : [...node.childNodes]).forEach(walk); return; }
        const isBlock = block.test(node.localName);
        if (isBlock) parts.push("\n");
        // a host renders its shadow tree; its own children appear only through that tree's slots
        (node.shadowRoot ? node.shadowRoot.childNodes : node.childNodes).forEach(walk);
        if (isBlock) parts.push("\n");
      };
      walk(root);
      return parts.join("").replace(/[ \t\r\f\v ]+/g, " ").replace(/ ?\n[\s]*/g, "\n").trim();
    };
    const deep = [];
    const collect = root => { for (const element of root.querySelectorAll("*")) { deep.push(element); if (element.shadowRoot) collect(element.shadowRoot); } };
    collect(document);
    if (deep.some(element => element.shadowRoot)) {
      const parent = element => element.parentElement || element.getRootNode()?.host || null;
      const shown = element => element.getClientRects().length && !element.closest(unrelated);
      const deepHeadline = deep.filter(element => element.localName === "h1" && shown(element)).sort((a, b) => headlineScore(b) - headlineScore(a))[0];
      let found = "";
      // the smallest container around the headline that holds the article, not just its byline
      for (let element = deepHeadline && headlineScore(deepHeadline) ? deepHeadline : null; element && !found; element = parent(element)) {
        const candidate = composedText(element);
        if (candidate.length >= 500) found = candidate;
      }
      if (!found && text.length < 200) {
        const largest = deep.filter(element => /^(article|main)$/.test(element.localName) && shown(element)).map(composedText).sort((a, b) => b.length - a.length)[0] || "";
        found = largest.length >= 200 ? largest : composedText(document.body);
      }
      // a headline-anchored article beats unanchored light-DOM text of any length
      if (found.length >= 200 && (deepHeadline && headlineScore(deepHeadline) || found.length > text.length)) text = found;
    }
  }
  const clone = document.documentElement.cloneNode(true);
  clone.querySelectorAll("script, style, noscript, svg").forEach(element => element.remove());
  return { text, content: "<!doctype html>\n" + clone.outerHTML, metadata, height: document.documentElement.scrollHeight };
}

const WEBP_MAX_DIMENSION = 16383;

// The photos in the article: large enough to be photos, not in the page's header, menu, sidebar or
// footer, each once, at most eight. Only their addresses, captions and credits: the research agent
// may link them as a report's media; no copy is kept.
async function captureImages(page) {
  const found = await page.evaluate(() => {
    const clean = (v, max = 400) => String(v || "").replace(/\s+/g, " ").trim().slice(0, max);
    // the story: the article or main around the headline (a page may have small <article> boxes
    // elsewhere); when that holds no photo, the whole page
    const story = document.querySelector("h1")?.closest("article, main") || document.querySelector("main") || document.body;
    const big = (img) => { const r = img.getBoundingClientRect(); return r.width >= 280 && r.height >= 160; };
    const scope = [...story.querySelectorAll("img")].some(big) ? story : document.body;
    const seen = new Set(), out = [];
    for (const img of scope.querySelectorAll("img")) {
      const r = img.getBoundingClientRect(), src = img.currentSrc || img.src || "";
      if (r.width < 280 || r.height < 160 || !/^https?:/.test(src) || seen.has(src)) continue;
      if (img.closest("header, nav, aside, footer, [role=banner], [role=navigation], [role=complementary], [class*=related], [class*=recommend], [class*=promo], [class*=advert]")) continue;
      seen.add(src);
      const figure = img.closest("figure");
      const caption = clean(figure?.querySelector("figcaption")?.innerText || img.alt || img.title);
      const credit = clean((caption.match(/(?:foto|photo|zdroj|credit|source|©)\s*[:©]?\s*([^|·\n]{2,80})$/i) || [])[1], 120);
      out.push({ src: src.slice(0, 2000), caption, credit, width: Math.round(r.width), height: Math.round(r.height) });
      if (out.length >= 8) break;
    }
    return out;
  });
  return found.map(({ src, caption, credit, width, height }) => ({ src, caption, credit, width, height }));
}

export async function captureSource({ url, dir, proxy, token }) {
  const imageBuffers = new Map();
  const result = await capturePage({ url, dir, proxy, token, imageBuffers });
  // Encode after closing Chromium and before handing the capture to consumers.
  const { evidence } = result;
  const signal = AbortSignal.timeout(8000);
  const save = async file => {
    const target = path.join(dir, file);
    if (file.endsWith(".png")) return path.relative(dir, await saveCaptureImage(target, imageBuffers.get(file), signal));
    await fs.writeFile(target, imageBuffers.get(file), { mode: 0o600 });
    return file;
  };
  if (evidence.screenshot) evidence.screenshot = await save(evidence.screenshot);
  await fs.writeFile(path.join(dir, "metadata.json"), JSON.stringify(evidence.metadata, null, 2), { mode: 0o600 });
  return result;
}

async function capturePage({ url, dir, proxy, token, imageBuffers }) {
  if (!proxy || !token || !/^http:\/\/127\.0\.0\.1:\d+$/.test(proxy)) throw new Error("Source proxy is unavailable.");
  const blocker = await loadBlocker();
  let blocked = 0, page;
  const browser = await chromium.launch({ channel: "chromium", headless: true, timeout: 12000, chromiumSandbox: true,
    // everything, loopback included (<-loopback>), goes through the checking proxy, with this
    // capture's password
    proxy: { server: proxy, username: "capture", password: token },
    args: ["--proxy-bypass-list=<-loopback>", "--disable-background-networking", "--disable-quic", "--force-webrtc-ip-handling-policy=disable_non_proxied_udp"],
    env: { PATH: process.env.PATH || "", HOME: process.env.HOME || "", TMPDIR: process.env.TMPDIR || "" },
  });
  try {
    // a browser's own user agent, without the "HeadlessChrome" that headless mode announces
    const userAgent = `Mozilla/5.0 (${process.platform === "darwin" ? "Macintosh; Intel Mac OS X 10_15_7" : "X11; Linux x86_64"}) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${browser.version().split(".")[0]}.0.0.0 Safari/537.36`;
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, userAgent, serviceWorkers: "block", acceptDownloads: false, ignoreHTTPSErrors: false });
    await context.routeWebSocket("**/*", socket => socket.close());
    await context.route("**/*", async route => {
      const request = route.request();
      if (!/^https?:\/\//i.test(request.url()) || request.resourceType() === "media") { await route.abort(); return; }
      // ads and trackers by the filter lists; never the page itself
      if (blocker && !(request.isNavigationRequest() && request.frame() === page?.mainFrame())) {
        try { if (blocker.match(fromPlaywrightDetails(request)).match) { blocked++; if (process.env.SOURCE_BLOCKER_DEBUG) console.error("blocked", request.resourceType(), request.url()); await route.abort("blockedbyclient"); return; } } catch { /* a request without a frame: let it through */ }
      }
      // the rest goes out from Chromium itself, through the proxy
      await route.continue().catch(() => {});
    });
    page = await context.newPage();
    page.on("dialog", dialog => dialog.dismiss());
    // the lists' hiding rules and scriptlets in every frame (only the DOM: requests stay on our route)
    if (blocker) page.on("framenavigated", (frame) => { blocker.onFrameNavigated(frame).catch(() => {}); });
    const consent = await enableAutoconsent(page);
    context.on("page", popup => { if (popup !== page) popup.close().catch(() => {}); });
    let response, navigationError = "";
    // A challenge may navigate to the real article while the capture settles.
    page.on("response", result => { if (result.request().isNavigationRequest() && result.frame() === page.mainFrame()) response = result; });
    try { response = await page.goto(url, { waitUntil: "domcontentloaded", timeout: 25000 }); }
    catch (error) { navigationError = String(error.message).split("\n")[0].slice(0, 300); }
    await page.waitForLoadState("networkidle", { timeout: 5000 }).catch(() => {});
    await page.waitForFunction(() => (document.body?.innerText || "").trim().length >= 200, null, { timeout: 6000 }).catch(() => {});
    // a consent dialog Autoconsent found: give it time to refuse and close it
    for (let waited = 0; waited < 8000 && !consent.done && (consent.detected || waited < 2000); waited += 250) await page.waitForTimeout(250);
    // Sample text until it settles, bounded independently of long polling/analytics.
    let previous = "";
    for (let i = 0; i < 4; i++) {
      const current = await page.locator("body").innerText({ timeout: 2000 }).catch(() => "");
      if (current && current === previous) break;
      previous = current; await page.waitForTimeout(400);
    }
    const extracted = await page.evaluate(extractSourcePage);
    if (Buffer.byteLength(extracted.text) > 2 * 1024 * 1024 || Buffer.byteLength(extracted.content) > 2 * 1024 * 1024) throw new Error("The rendered source is larger than 2 MB.");
    // The article's own photos, each saved as it is shown on the page (an element screenshot: nothing
    // is fetched again), with its caption and credit. They are kept in the private capture, shown on
    // the local request page and offered to the researcher, which may link them as the report's
    // media; the site itself never copies a newspaper's picture (docs/WRITING.md §7).
    extracted.metadata.images = await captureImages(page).catch(() => []);
    // The whole page, top to bottom, so nothing on it is lost (images of text, tables, comments):
    // scroll through it first so lazy images load, then one full-page screenshot, capped only for
    // pages that never end
    const MAX_HEIGHT = 40000;
    await page.evaluate(async (max) => {
      const step = Math.max(600, innerHeight - 100);
      for (let y = 0; y < Math.min(document.documentElement.scrollHeight, max); y += step) { scrollTo(0, y); await new Promise((ok) => setTimeout(ok, 120)); }
      scrollTo(0, 0);
    }, MAX_HEIGHT).catch(() => {});
    await page.waitForLoadState("networkidle", { timeout: 3000 }).catch(() => {});
    const { fullHeight, fullWidth } = await page.evaluate(() => ({
      fullHeight: document.documentElement.scrollHeight,
      fullWidth: document.documentElement.scrollWidth,
    })).catch(() => ({ fullHeight: extracted.height, fullWidth: 1440 }));
    const height = Math.max(900, Math.min(fullHeight, MAX_HEIGHT));
    let screenshot = "", screenshotError = "";
    try {
      // WebP cannot encode dimensions over 16,383 pixels. For larger pages,
      // send a PNG buffer to the AVIF encoder so it can resize the whole page.
      const fullPage = fullHeight <= MAX_HEIGHT;
      const type = height <= WEBP_MAX_DIMENSION && (!fullPage || fullWidth <= WEBP_MAX_DIMENSION) ? "webp" : "png";
      const image = await page.screenshot({ type, ...(fullPage ? { fullPage: true } : { clip: { x: 0, y: 0, width: 1440, height } }), animations: "disabled", timeout: 30000 });
      screenshot = `page.${type}`;
      imageBuffers.set(screenshot, image);
    } catch { screenshotError = "The page rendered, but its screenshot could not be captured."; }
    await Promise.all([
      fs.writeFile(path.join(dir, "page.html"), extracted.content, { mode: 0o600 }),
      fs.writeFile(path.join(dir, "page.txt"), extracted.text, { mode: 0o600 }),
    ]);
    const status = response?.status() || 0;
    const evidence = { access_blocked: (await response?.headerValue("cf-mitigated").catch(() => "")) === "challenge" ? "Cloudflare" : "", url, final_url: page.url(), content: extracted.content, text: extracted.text, metadata: extracted.metadata, rendered: true, screenshot, browser_warning: screenshotError,
      cleanup: { blocker: Boolean(blocker), blocked_requests: blocked, consent: consent.done ? `refused (${consent.cmp || "dialog"})` : consent.detected ? `${blocker ? "hidden" : "found, not answered"} (${consent.cmp || "dialog"})` : "none found" } };
    if (height < fullHeight) evidence.browser_warning = [screenshotError, "Screenshot height limited to 40,000 pixels."].filter(Boolean).join(" ");
    const error = status >= 400 ? `The browser page answered HTTP ${status}.` : navigationError && !extracted.text ? "Browser navigation failed." : "";
    return { evidence, error, status };
  } finally { await browser.close(); }
}

async function main() {
  const [url, dir] = process.argv.slice(2);
  try { const result = await captureSource({ url, dir, proxy: process.env.SOURCE_PROXY, token: process.env.SOURCE_PROXY_TOKEN }); process.stdout.write(JSON.stringify(result)); }
  catch (error) {
    const missing = /Executable doesn't exist|browserType.launch|playwright install/.test(String(error));
    process.stdout.write(JSON.stringify({ error: missing ? "Playwright Chromium is unavailable. Run npm run browser-install." : String(error.message).split("\n")[0].slice(0, 300), unavailable: missing })); process.exitCode = 1;
  }
}
if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) main();
