// A research worker on this machine for the tracker backend on the server (docs/WORKERS.md):
// `npm run worker`. It reads the root .env, like backend-dev, plus TRACKER_URL and WORKER_TOKEN.
import { loadEnvFile } from "node:process";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

try { loadEnvFile(new URL("../.env", import.meta.url)); }
catch (error) { if (error.code !== "ENOENT") throw error; }
const env = { ...process.env };
env.TRACKER_URL ||= "https://tracker-api.cannabisfreedom.fyi";
env.RESEARCH_RUNNER ||= "local";
// this checkout, or a clone of its own (WORKER_REPO_PATH); fast-forwarded before every job
env.REPO_PATH = env.WORKER_REPO_PATH || fileURLToPath(new URL("..", import.meta.url));
env.WORKSPACE_PATH ||= "data/worker-jobs";
if (!env.WORKER_TOKEN) {
  console.error("Set WORKER_TOKEN in .env: the token of this worker in the backend's WORKER_TOKENS.");
  process.exit(1);
}
const child = spawn("go", ["run", "./cmd/tracker-worker", ...process.argv.slice(2)], { cwd: fileURLToPath(new URL("../backend/", import.meta.url)), env, stdio: "inherit" });
child.on("error", () => { console.error("Could not start the worker. Check that Go is installed."); process.exitCode = 1; });
child.on("exit", (code, signal) => { process.exitCode = signal ? 0 : code ?? 1; });
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => child.kill(signal));
