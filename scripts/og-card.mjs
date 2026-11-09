// The site's share card, public/og.png (1200×630): what links to pages without their own picture
// show on social sites and in chats. It names no figures, so it never goes stale.
// Run: node scripts/og-card.mjs
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";

const out = fileURLToPath(new URL("../public/og.png", import.meta.url));
const html = `<!doctype html><html><head><meta charset="utf-8"><style>
  * { margin: 0; box-sizing: border-box; }
  body { width: 1200px; height: 630px; background: #0c0c0b; color: #f2f0e9; font-family: Inter, "Helvetica Neue", Arial, sans-serif;
    padding: 64px 80px 56px; display: flex; flex-direction: column; position: relative; overflow: hidden; }
  .grid { position: absolute; inset: 0; background-image: radial-gradient(#2a2925 1.4px, transparent 1.6px); background-size: 22px 22px; opacity: .9;
    mask-image: linear-gradient(105deg, transparent 35%, #000 80%); }
  .brand { display: flex; align-items: center; gap: 18px; font-size: 26px; color: #b9b6aa; position: relative; }
  .mark { width: 52px; height: 52px; border-radius: 3px; background: #f2f0e9; color: #0c0c0b; display: grid; place-items: center; font-weight: 700; font-size: 28px; }
  h1 { font-family: "Instrument Serif", "Iowan Old Style", Georgia, serif; font-weight: 400; font-size: 84px; line-height: 1.02; letter-spacing: -.02em;
    margin-top: 48px; position: relative; max-width: 1040px; }
  p { font-size: 29px; line-height: 1.35; color: #b9b6aa; margin-top: 26px; max-width: 860px; position: relative; }
  footer { margin-top: auto; display: flex; justify-content: space-between; font-size: 22px; color: #77756c; position: relative;
    border-top: 1px solid #2a2925; padding-top: 24px; }
</style></head><body><div class="grid"></div>
  <div class="brand"><span class="mark">✕</span>Cannabis Enforcement Tracker</div>
  <h1>Cannabis enforcement, on the record</h1>
  <p>Eradication, seizures, raids, arrests, prosecutions, sentences and executions around the world. Every report sourced.</p>
  <footer><span>tracker.cannabisfreedom.fyi</span><span>A companion to the Cannabis Freedom Index</span></footer>
</body></html>`;

const browser = await chromium.launch({ channel: "chromium", headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1200, height: 630 } });
  await page.setContent(html);
  await page.screenshot({ path: out, type: "png" });
  console.log(out);
} finally { await browser.close(); }
