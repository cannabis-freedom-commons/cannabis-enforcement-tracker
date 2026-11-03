import { load, JSON_SCHEMA } from "js-yaml";

import reportAliases from "./report-aliases.js";

const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const SAVED = new Set(["committed", "published", "duplicate"]);

export function savedReportPath(v, base = "/") {
  return SAVED.has(v?.status) && SLUG.test(v.report_slug || "") ? `${base}report/${reportAliases.slugs[v.report_slug] || v.report_slug}/` : null;
}

// Once prepared, the actual report file is authoritative, including an empty
// media list after deduplication. Source-capture images are research evidence,
// never an automatic selection of report illustrations.
export function previewMedia(v) {
  if (v?.draft_markdown) {
    try {
      const front = v.draft_markdown.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/)?.[1];
      const data = front ? load(front, { schema: JSON_SCHEMA }) : null;
      return { media: Array.isArray(data?.media) ? data.media : [], cover: data?.cover ?? null };
    } catch { return { media: [], cover: null }; }
  }
  return { media: Array.isArray(v?.result?.media) ? v.result.media : [], cover: null };
}
