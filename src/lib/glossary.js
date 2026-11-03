// The same sourced definitions as CFI, synced explicitly for offline static builds.
import data from "../data/glossary.json" with { type: "json" };
import { measureMatches } from "./measures.js";

const skip = new Set(["bang", "hemp", "hanf", "flower", "jay", "j", "tea", "viper", "hybrid", "weed", "hash", "accession", "novel food", "hemp extract", "doped", "dispo", "maría", "juana", "pacheco", "toque", "churro", "grifo", "moto", "beng", "sır", "ciga", "chimique", "candia", "hindu kush", "roach", "mezz", "d8", "geras", "alaf", "sensi", "sinse"]);
const clean = value => String(value ?? "").replace(/\s*\(.*\)$/, "").trim();
const escape = text => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const owner = new Map();
const titles = new Set(data.entries.map(entry => clean(entry.id).toLowerCase()));
for (const entry of [...data.entries.filter(e => e.kind === "language"), ...data.entries.filter(e => e.kind !== "language")]) for (const form of [entry.id, ...entry.also]) {
  const word = clean(form);
  if (word.length < 2 || skip.has(word.toLowerCase())) continue;
  if (titles.has(word.toLowerCase()) && clean(entry.id).toLowerCase() !== word.toLowerCase()) continue;
  const key = word.length < 4 ? word : word.toLowerCase();
  if (!owner.has(key)) owner.set(key, { word, entry });
}
const forms = [...owner.values()].sort((a,b) => b.word.length - a.word.length);
const pattern = words => `(^|[^\\p{L}\\p{N}])(${words.map(escape).join("|")})(?![\\p{L}\\p{N}])`;
const regexes = [
  forms.filter(({word}) => word.length >= 4).map(({word}) => word),
  forms.filter(({word}) => word.length < 4).map(({word}) => word),
].map((words,i) => words.length ? new RegExp(pattern(words), i === 0 ? "giu" : "gu") : null).filter(Boolean);

// The glossary's terms and the text's weights and sums of money (measures.js, which come first:
// "Rs 60.48 lakh" is one amount, not the term "lakh"). context.country reads "$" or "Rs" as that
// country's currency. One explanation per term in a text/page; every weight and sum of money gets
// its own, wherever it is read (a summary repeats the text's figures).
export function glossaryParts(text, seen = new Set(), context = {}) {
  const s = String(text ?? ""), parts = [];
  let last = 0;
  for (const m of measureMatches(s, context)) {
    parts.push(...termParts(s.slice(last, m.at), seen), { text: m.text, entry: m.entry });
    last = m.at + m.text.length;
  }
  parts.push(...termParts(s.slice(last), seen));
  return parts;
}

// Longest form at the earliest position; one explanation per canonical term in a text/page.
function termParts(text, seen) {
  const parts = []; let rest = String(text ?? "");
  for (;;) {
    let best;
    for (const re of regexes) {
      re.lastIndex = 0;
      for (let match; (match = re.exec(rest));) {
        const entry = owner.get(match[2].length < 4 ? match[2] : match[2].toLowerCase())?.entry;
        if (!entry || seen.has(entry.id)) continue;
        const at = match.index + match[1].length;
        if (!best || at < best.at || (at === best.at && match[2].length > best.text.length)) best = { at, text: match[2], entry };
        break;
      }
    }
    if (!best) break;
    if (best.at) parts.push({ text: rest.slice(0,best.at) });
    parts.push({ text: best.text, entry: best.entry }); seen.add(best.entry.id);
    rest = rest.slice(best.at + best.text.length);
  }
  if (rest) parts.push({ text: rest });
  return parts;
}

// Shared by the Markdown renderer and the browser's static/live prose highlighter.
export const GLOSSARY_SKIP = "a,button,input,textarea,select,option,label,nav,h1,h2,h3,h4,h5,h6,pre,code,kbd,samp,script,style,noscript,svg,canvas,sup,figcaption,.term,.ai-chat,[role=log],[contenteditable],[data-map],[data-no-glossary]";
export function markdownGlossary() {
  return (tree, file) => {
    const seen = new Set(), context = { country: file?.data?.astro?.frontmatter?.country };
    const skipTags = new Set(GLOSSARY_SKIP.split(",").filter(tag => /^[a-z][a-z0-9]*$/.test(tag)));
    function walk(node) {
      if (node.type === "element" && (skipTags.has(node.tagName) || node.properties?.className?.includes("term"))) return;
      if (!node.children) return;
      node.children = node.children.flatMap(child => {
        if (child.type !== "text") { walk(child); return [child]; }
        return glossaryParts(child.value, seen, context).map(part => part.entry ? {
          type: "element", tagName: part.entry.url ? "a" : "span", properties: { className: ["term", "term-g", ...(part.entry.url ? ["term-l"] : [])], tabIndex: 0, ...(part.entry.url ? {href: part.entry.url, target: "_blank", rel: "noopener noreferrer"} : {role: "button"}), dataTermId: part.entry.id, dataTitle: part.entry.title ?? part.entry.id, dataText: part.entry.text, ...(part.entry.eyebrow ? { dataEyebrow: part.entry.eyebrow } : {}), ...(part.entry.rows ? { dataRows: JSON.stringify(part.entry.rows), dataNote: part.entry.note } : {}) }, children: [{type:"text",value:part.text}],
        } : {type:"text",value:part.text});
      });
    }
    walk(tree);
  };
}
