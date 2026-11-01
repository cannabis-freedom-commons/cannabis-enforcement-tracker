// What a valid report looks like. The build fails with a clear message if a file in /reports breaks
// these rules: above all, a report without sources is refused.
import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";
import { z } from "astro/zod";
import countries from "./data/countries.json";
import { TYPE_IDS, PRECISION, STATUS } from "./lib/types.js";

const COUNTRY_IDS = countries.map((c) => c.id) as [string, ...string[]];
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
}).strict();

const reports = defineCollection({
  // reports/<year>/<month>/<slug>.md; the slug alone is the report's id and address. REPORTS_DIR
  // builds from another folder (scripts/load-test.mjs)
  loader: glob({ pattern: "**/*.md", base: process.env.REPORTS_DIR || "./reports", generateId: ({ entry }) => entry.split("/").pop()!.replace(/\.md$/, "") }),
  schema: ({ image }) => z.preprocess((value) => {
    // Older writers may still send this retired field; never expose it in report data.
    if (!value || typeof value !== "object" || Array.isArray(value)) return value;
    const { updated_by_models, ...report } = value as Record<string, unknown>;
    return report;
  }, z.object({
    code: z.string().regex(/^[0-9A-HJKMNP-TV-Z]{6}$/, { message: "six characters of Crockford base32; npm run codes writes it" }).optional(), // the permanent short id, CET-XXXXXX (scripts/codes.mjs)
    date: day, // when it happened (or, for a campaign, when it ended or was announced)
    country: z.enum(COUNTRY_IDS), // a country file of the Cannabis Freedom Index: "de", "us/ca"
    place: z.string().min(1).optional(),
    coordinates: z.tuple([z.number().min(-90).max(90), z.number().min(-180).max(180)]).optional(), // [lat, lon]
    precision: z.enum(Object.keys(PRECISION) as [string, ...string[]]).optional(),
    type: z.enum(TYPE_IDS as [string, ...string[]]),
    also: z.array(z.enum(TYPE_IDS as [string, ...string[]])).optional(), // other kinds it involved
    title: z.string().min(1).max(120),
    summary: z.string().min(1).max(400),
    figures: figures.optional(),
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
    open_questions: z.array(z.string().trim().min(1).max(4000)).max(30).optional(),
    sources: z.array(source).min(1, { message: "a report needs at least one source" }),
  }).strict()
    .refine((r) => !r.coordinates || r.precision, { message: "coordinates need a precision" })
    .refine((r) => !r.also?.includes(r.type), { message: "`also` repeats the main type" })),
});

export const collections = { reports };
