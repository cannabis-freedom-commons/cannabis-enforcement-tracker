// What a valid report looks like. The build fails with a clear message if a file in /reports breaks
// these rules: above all, a report without sources is refused.
import { defineCollection } from "astro:content";
import { reportLoader } from "../scripts/lib/report-loader.mjs";
import { z } from "astro/zod";
import countries from "./data/countries.json";
import vocabularies from "./data/vocabularies.json";
import agencies from "./data/agencies.json";
import { TYPE_IDS, PRECISION, STATUS } from "./lib/types.js";

const COUNTRY_IDS = countries.map((c) => c.id) as [string, ...string[]];
// the allowed values of the structured details (src/data/vocabularies.json)
const term = (list: keyof typeof vocabularies) => z.enum(vocabularies[list].map((t) => t.id) as [string, ...string[]]);
const terms = (list: keyof typeof vocabularies) => z.array(term(list)).min(1).max(10);
const AGENCY_IDS = new Set(agencies.map((a) => a.id));
const day = z.coerce.date();
const money = z.object({ amount: z.number().positive(), currency: z.string().regex(/^[A-Z]{3}$/) }).strict();

// A source is a web page, or a publication with no public address (a book, journal, report or law)
// described well enough to find it.
const source = z.object({
  title: z.string().min(1),
  url: z.string().url().optional(),
  publisher: z.string().min(1),
  type: z.enum(["book", "chapter", "journal", "report", "law", "other"]).optional(), // not a web page
  authors: z.array(z.string().min(1)).optional(),
  container: z.string().min(1).optional(), // the journal or book a chapter is in
  year: z.number().int().optional(),
  isbn: z.string().min(1).optional(),
  locator: z.string().min(1).optional(), // the page, section or paragraph
  date: day.optional(), // when the source was published
  archived: z.string().url().optional(), // a copy in the Wayback Machine or archive.today
  language: z.string().regex(/^[a-z]{2,3}$/).optional(), // ISO 639-1 or ISO 639-2 code, if not English
}).strict().refine((s) => s.url || s.type, { message: "a source needs a url, or a type (book, journal, report, law) with its details" });

// What the enforcement did, in numbers, where a source gives them. Everything is optional;
// a number goes in only if a source states it.
const figures = z.object({
  arrested: z.number().int().nonnegative().optional(), // people detained
  charged: z.number().int().nonnegative().optional(), // people charged or tried
  kg_seized: z.number().nonnegative().optional(), // cannabis taken or destroyed, in kilograms
  plants_destroyed: z.number().int().nonnegative().optional(),
  hectares: z.number().nonnegative().optional(), // fields destroyed
  prison: z.string().min(1).optional(), // a sentence as stated: "5 months 24 days", "life"
  fine: money.optional(),
  value: money.optional(), // the authorities' estimate of what was seized or destroyed
  deaths: z.number().int().nonnegative().optional(),
  // kg_seized and plants_destroyed stay the totals; these split them where a source does
  plants_seized: z.number().int().nonnegative().optional(), // taken away, not destroyed
  kg_destroyed: z.number().nonnegative().optional(), // burned or destroyed
  convicted: z.number().int().nonnegative().optional(),
  acquitted: z.number().int().nonnegative().optional(),
  sentenced: z.number().int().nonnegative().optional(),
  released: z.number().int().nonnegative().optional(), // freed without charge or on bail
  injured: z.number().int().nonnegative().optional(),
  sites: z.number().int().nonnegative().optional(), // places raided or plantations destroyed
  seeds: z.number().int().nonnegative().optional(),
  vehicles: z.number().int().nonnegative().optional(), // confiscated
  cash: money.optional(), // money confiscated
}).strict();

// Who acted, what was taken, where, and how far the case went: each value supported by a quoted
// source (data_sources), all optional. Ids come from src/data/vocabularies.json and agencies.json.
const details = z.object({
  agencies: z.array(z.string().min(2).max(100).refine((a) => AGENCY_IDS.has(a) || !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(a), { message: "an agency is a registry id (src/data/agencies.json) or a name as published" })).min(1).max(10).optional(),
  agency_types: terms("agency_type").optional(),
  products: z.array(z.object({ product: term("product"), kg: z.number().nonnegative().optional(), plants: z.number().int().nonnegative().optional() }).strict()).min(1).max(10).optional(),
  activity: terms("activity").optional(),
  setting: terms("setting").optional(),
  environment: terms("environment").optional(),
  charges: z.array(z.string().min(3).max(160)).min(1).max(10).optional(),
  case_stage: term("case_stage").optional(),
  sentence_types: terms("sentence_type").optional(),
  force: terms("force").optional(),
  period: z.object({ start: day, end: day }).strict().refine((p) => p.end >= p.start, { message: "period ends before it starts" }).optional(),
  date_precision: term("date_precision").optional(),
  region: z.string().min(1).max(100).optional(),
  route: z.object({ from: z.array(z.enum(COUNTRY_IDS)).optional(), to: z.array(z.enum(COUNTRY_IDS)).optional(), crossing: z.string().min(1).max(100).optional() }).strict().optional(),
  part_of: z.string().regex(/^[a-z0-9]+(-[a-z0-9]+)*$/).optional(), // a campaign report's slug
  related: z.array(z.string().regex(/^[a-z0-9]+(-[a-z0-9]+)*$/)).min(1).max(10).optional(),
}).strict();

const reports = defineCollection({
  // reports/<year>/<month>/<slug>.md; the slug alone is the report's id and address. REPORTS_DIR
  // builds from another folder (scripts/load-test.mjs)
  loader: reportLoader({ pattern: "**/*.md", base: process.env.REPORTS_DIR || "./reports", generateId: ({ entry }) => entry.split("/").pop()!.replace(/\.md$/, "") }),
  schema: ({ image }) => z.preprocess((value) => {
    // Older writers may still send this retired field; never expose it in report data.
    if (!value || typeof value !== "object" || Array.isArray(value)) return value;
    const { updated_by_models, ...report } = value as Record<string, unknown>;
    return report;
  }, z.object({
    code: z.string().regex(/^[0-9A-HJKMNP-TV-Z]{6}$/, { message: "six characters of Crockford base32; npm run codes writes it" }), // required permanent short id, CET-XXXXXX (scripts/codes.mjs)
    added_at_source: z.enum(["recorded", "git"]).optional(), // timestamp origin
    added_at: z.string().datetime().optional(), // UTC timestamp when first accepted into the tracker; immutable
    date: day, // when it happened (or, for a campaign, when it ended or was announced)
    country: z.enum(COUNTRY_IDS), // a country file of the Cannabis Freedom Index: "de", "us/ca"
    place: z.string().min(1).optional(),
    coordinates: z.tuple([z.number().min(-90).max(90), z.number().min(-180).max(180)]).optional(), // [lat, lon]
    precision: z.enum(Object.keys(PRECISION) as [string, ...string[]]).optional(),
    type: z.enum(TYPE_IDS as [string, ...string[]]),
    also: z.array(z.enum(TYPE_IDS as [string, ...string[]])).optional(), // other kinds it involved
    title: z.string().min(1).max(120).refine((t) => !t.includes("[@"), { message: "no citations ([@n]) in the title" }),
    summary: z.string().min(1).max(400).refine((t) => !t.includes("[@"), { message: "no citations ([@n]) in the summary; cite in the text" }),
    figures: figures.optional(),
    details: details.optional(),
    // which sources (by number) support each structured value: { "figures.arrested": [1] }
    data_sources: z.record(z.string().regex(/^(figures|details)\.[a-z_]+$/), z.array(z.number().int().positive()).min(1)).optional(),
    cover: z.object({
      image: image(),
      alt: z.string().min(1),
      credit: z.string().min(1),
      licence: z.string().min(1),
      url: z.string().url(), // where the picture and its licence can be checked
      media_url: z.string().url().optional(), // original photo URL; optional for legacy covers
    }).strict().optional(),
    // photos or video of the event that may not be copied here (a newspaper's or an agency's):
    // shown as links to where they are published
    media: z.array(z.object({
      kind: z.enum(["photo", "video"]),
      url: z.string().url(), // source page
      media_url: z.string().url().optional(), // required for newly collected media; optional for legacy entries
      publisher: z.string().min(1),
      caption: z.string().min(1), // what it shows
      archived: z.string().url().optional(), // a copy in the Wayback Machine
    }).strict()).optional(),
    status: z.enum(Object.keys(STATUS) as [string, ...string[]]),
    cfi_id: z.string().regex(/^[a-z0-9-]+$/).optional(), // the Cannabis Freedom Index history entry it was moved from
    tags: z.array(z.string().regex(/^[a-z0-9-]+$/)).optional(),
    created_by_model: z.string().trim().min(1).max(300), // required; "unknown" only when attribution was not recorded
    submission: z.string().regex(/^[0-9a-f]{32}$/).optional(), // the submission the service created it from
    // Updates accumulate questions from many sources; the model's per-run limit is separate.
    open_questions: z.array(z.string().trim().min(1).max(4000)).optional(),
    sources: z.array(source).min(1, { message: "a report needs at least one source" }),
  }).strict()
    .refine((r) => !r.added_at_source || r.added_at, { message: "added_at_source needs added_at" })
    .refine((r) => !r.coordinates || r.precision, { message: "coordinates need a precision" })
    .refine((r) => !r.also?.includes(r.type), { message: "`also` repeats the main type" })
    .refine((r) => Object.entries(r.data_sources ?? {}).every(([field, list]) => {
      const [group, key] = field.split(".");
      return (r as any)[group]?.[key] != null && list.every((n) => n <= r.sources.length);
    }), { message: "data_sources names a value the report does not give, or a source it does not list" })),
});

export const collections = { reports };
