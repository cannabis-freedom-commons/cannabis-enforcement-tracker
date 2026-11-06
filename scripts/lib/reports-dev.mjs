// The dev server's report refresh. Report files are not watched (research writes thousands of them
// mid-run); the reports reload from the header's "Refresh reports" button, and on their own when a
// new commit changes reports (a published submission, a skill's commit). Either way Astro reloads
// open pages, keeping their address and filters.
import { execFile } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { promisify } from "node:util";

const git = promisify(execFile);
const REPORT_PATHS = ["reports", "src/assets/reports"];

// the reports a commit range changed, or null when Git cannot tell
export async function reportsChanged(root, from, to) {
  try {
    const { stdout } = await git("git", ["diff", "--name-only", from, to, "--", ...REPORT_PATHS], { cwd: root });
    return stdout.split("\n").filter(Boolean);
  } catch { return null; }
}

export function reportsDev({ root, interval = 2000 } = {}) {
  return {
    name: "tracker:reports-dev",
    hooks: {
      "astro:server:setup"({ server, refreshContent, logger }) {
        let pending;
        root ??= server.config.root;
        const base = server.config.base.replace(/\/$/, "");
        const refresh = () => {
          pending ??= (async () => {
            await refreshContent({ loaders: ["tracker-reports"] });
            // Invalidate computed indexes, Git history and Astro's static path cache
            // after the content store is persisted.
            for (const environment of Object.values(server.environments)) {
              environment.moduleGraph.invalidateAll();
              environment.hot.send("astro:content-changed", {});
            }
          })().finally(() => { pending = undefined; });
          return pending;
        };
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
            await refresh(); // the requesting browser reloads
            response.statusCode = 204;
            response.end();
          } catch (error) {
            logger.error(`Could not refresh reports: ${error.message}`);
            response.statusCode = 500;
            response.end("Could not refresh reports. Check the dev server output.");
          }
        });

        // A new commit: Git appends to its HEAD log. Only a commit that changes reports refreshes.
        (async () => {
          let head, log;
          try {
            head = (await git("git", ["rev-parse", "HEAD"], { cwd: root })).stdout.trim();
            log = path.resolve(root, (await git("git", ["rev-parse", "--git-path", "logs/HEAD"], { cwd: root })).stdout.trim());
          } catch { return; } // not a Git checkout: the button only
          let busy = false;
          const onCommit = async () => {
            if (busy) return;
            busy = true;
            try {
              const now = (await git("git", ["rev-parse", "HEAD"], { cwd: root })).stdout.trim();
              if (now === head) return;
              const changed = await reportsChanged(root, head, now);
              head = now;
              if (changed && changed.length === 0) return;
              await refresh();
              const reports = fs.globSync("**/*.md", { cwd: path.resolve(root, process.env.REPORTS_DIR || "reports") }).length;
              logger.info(`Reports refreshed after commit ${now.slice(0, 9)}${changed ? ` (${changed.length} files changed)` : ""}`);
              server.hot.send("tracker:reports-refreshed", { id: now, reports });
            } catch (error) {
              logger.error(`Could not refresh reports after a commit: ${error.message}`);
            } finally { busy = false; }
          };
          fs.watchFile(log, { interval }, onCommit);
          server.httpServer?.once("close", () => fs.unwatchFile(log, onCommit));
        })();
      },
    },
  };
}
