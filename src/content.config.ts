// What a valid report looks like. The build fails with a clear message if a file in /reports breaks
// these rules: above all, a report without sources is refused.
import { defineCollection } from "astro:content";
import { reportLoader } from "../scripts/lib/report-loader.mjs";
import { reportSchema } from "./lib/report-schema.ts";

const reports = defineCollection({
  // reports/<year>/<month>/<slug>.md; the slug alone is the report's id and address. REPORTS_DIR
  // builds from another folder (scripts/load-test.mjs)
  loader: reportLoader({ pattern: "**/*.md", base: process.env.REPORTS_DIR || "./reports", generateId: ({ entry }) => entry.split("/").pop()!.replace(/\.md$/, "") }),
  schema: reportSchema,
});

export const collections = { reports };
