import { glob } from "astro/loaders";

// The dev header explicitly syncs this loader. Do not attach file listeners:
// the backend writes reports continuously, independently of frontend edits.
export function reportLoader(options) {
  const loader = glob(options);
  return {
    ...loader,
    name: "tracker-reports",
    load: (context) => loader.load({ ...context, watcher: undefined }),
  };
}
