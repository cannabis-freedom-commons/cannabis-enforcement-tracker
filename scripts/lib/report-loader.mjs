import { glob } from "astro/loaders";

// How many times the loader has synced, in this process: the dev refresh (reports-dev.mjs) checks
// that its sync really ran, as Astro skips one while its content configuration is reloading.
export const SYNCS = Symbol.for("tracker.reportSyncs");

// The dev header explicitly syncs this loader. Do not attach file listeners:
// the backend writes reports continuously, independently of frontend edits.
export function reportLoader(options) {
  const loader = glob(options);
  return {
    ...loader,
    name: "tracker-reports",
    load: (context) => {
      globalThis[SYNCS] = (globalThis[SYNCS] ?? 0) + 1;
      return loader.load({ ...context, watcher: undefined });
    },
  };
}
