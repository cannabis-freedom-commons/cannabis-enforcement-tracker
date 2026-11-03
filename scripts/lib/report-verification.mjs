import { isDeepStrictEqual } from "node:util";
import * as yaml from "js-yaml";

const parse = (text) => {
  const match = text.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  if (!match) throw new Error("missing front matter");
  return { header: yaml.load(match[1], { schema: yaml.JSON_SCHEMA }), body: match[2] };
};

// Only service-owned checks for an unchanged, complete new report can certify older prose.
// A successful update validates its additions, not all the earlier sentences it retains.
export function recheckReportStatus(text, record) {
  const current = parse(text);
  const reject = (reason) => ({ eligible: false, reason });
  if (current.header.status !== "reported") return reject("already verified or disputed");
  if (typeof current.header.code !== "string" || !/^[0-9A-HJKMNP-TV-Z]{6}$/.test(current.header.code)) return reject("invalid code");
  let result, check, saved;
  try {
    result = JSON.parse(record.ai_result);
    check = JSON.parse(record.validation);
    saved = parse(record.draft_markdown);
  } catch { return reject("missing validation or draft"); }
  if (!result || !check) return reject("missing validation or draft");
  if (result.outcome !== "new_event") return reject("update does not verify older prose");
  if (result.proposed_slug !== record.report_slug) return reject("slug mismatch");
  if (!Array.isArray(result.facts) || !result.facts.length || !Array.isArray(result.sources) || !Array.isArray(check.sources)) return reject("missing facts or source checks");
  if (check.passed !== true || check.confirmed !== result.facts.length) return reject("retained claims were not all confirmed");
  const counts = new Map();
  for (const fact of result.facts) {
    if (!Number.isInteger(fact.source) || fact.source < 1 || fact.source > result.sources.length) return reject("invalid source reference");
    counts.set(fact.source, (counts.get(fact.source) ?? 0) + 1);
  }
  for (const [source, count] of counts) {
    const matches = check.sources.filter((s) => s.url === result.sources[source - 1].url);
    if (matches.length !== 1 || matches[0].opened !== true || matches[0].confirmed !== count) return reject("incomplete source checks");
  }
  const normalize = (value) => value.replace(/\s+/g, " ").trim();
  const expectedBody = result.facts.map((fact) => `${fact.text} [@${fact.source}]`).join(" ");
  if (normalize(current.body) !== normalize(expectedBody)) return reject("draft facts do not match checked result");
  if (!isDeepStrictEqual(current.header.sources?.map((source) => source.url), result.sources.map((source) => source.url))) return reject("draft sources do not match checked result");
  const { code, status, ...header } = current.header;
  const { code: savedCode, status: savedStatus, ...savedHeader } = saved.header;
  if (savedCode != null && savedCode !== code) return reject("code changed since validation");
  if (!isDeepStrictEqual(header, savedHeader) || current.body !== saved.body) return reject("report changed since validation");
  if (!/^status: (?:reported|'reported'|"reported")$/m.test(text)) return reject("unsupported status layout");
  return { eligible: true, reason: "all retained claims confirmed", content: text.replace(/^status: (?:reported|'reported'|"reported")$/m, "status: verified") };
}
