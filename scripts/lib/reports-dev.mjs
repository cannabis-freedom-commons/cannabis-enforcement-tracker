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
              await refreshContent({ loaders: ["tracker-reports"] });
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
