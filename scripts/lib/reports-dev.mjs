// What the dev server does not watch: files the backend and research change all the time. A change
// to them would re-run the report indexes and reload every open page (it freezes the site for a
// while), so they load from the header's "Refresh reports" button instead, with the reports:
// report files, their cover images, and the photo manifest each "Add photo thumbnails" commit
// rewrites (src/data/media.json, imported by src/lib/reports.js).
// the loader's sync count (report-loader.mjs), shared in the process
const SYNCS = Symbol.for("tracker.reportSyncs");

export function devWatchIgnored(reportsDirectory) {
  return ["**/backend/**", "**/.cache/**", "**/src/assets/reports/**", "**/src/data/media.json", "**/.git/**", `${reportsDirectory}/**`];
}

export function reportsDev() {
  return {
    name: "tracker:reports-dev",
    hooks: {
      "astro:server:setup"({ server, refreshContent, logger }) {
        let pending;
        const base = server.config.base.replace(/\/$/, "");
        server.middlewares.use(async (request, response, next) => {
          if (request.url?.split("?")[0] !== `${base}/__dev/refresh-reports`) return next();
          response.setHeader("Cache-Control", "no-store");
          if (request.method !== "POST") {
            response.setHeader("Allow", "POST");
            response.statusCode = 405;
            return response.end();
          }
          // Only the dev UI may request a sync, including when dev is LAN-accessible.
          if (request.headers.origin && request.headers.origin !== `http://${request.headers.host}` && request.headers.origin !== `https://${request.headers.host}`) {
            response.statusCode = 403;
            return response.end();
          }
          try {
            pending ??= (async () => {
              // Astro returns without syncing while its content configuration reloads (each refresh
              // invalidates every module, so it does after one): ask again until the loader ran
              const before = globalThis[SYNCS] ?? 0, deadline = Date.now() + 60000;
              await refreshContent({ loaders: ["tracker-reports"] });
              while ((globalThis[SYNCS] ?? 0) === before) {
                if (Date.now() > deadline) throw new Error("the content configuration did not finish loading");
                await new Promise((resolve) => setTimeout(resolve, 250));
                await refreshContent({ loaders: ["tracker-reports"] });
              }
              // Invalidate computed indexes, Git history and Astro's static path cache
              // after the content store is persisted. The requesting browser reloads.
              for (const environment of Object.values(server.environments)) {
                environment.moduleGraph.invalidateAll();
                environment.hot.send("astro:content-changed", {});
              }
            })().finally(() => { pending = undefined; });
            await pending;
            response.statusCode = 204;
            response.end();
          } catch (error) {
            logger.error(`Could not refresh reports: ${error.message}`);
            response.statusCode = 500;
            response.end("Could not refresh reports. Check the dev server output.");
          }
        });
      },
    },
  };
}
