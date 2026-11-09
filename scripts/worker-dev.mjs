// A research worker on this machine for the tracker backend on the server (docs/WORKERS.md):
// `npm run worker`. It reads the root .env, like backend-dev, plus TRACKER_URL and WORKER_TOKEN.
//
// The worker researches against a clone of its own (backend/data/worker-repo, or WORKER_REPO_PATH),
// which it resets before every job to the backend's publish branch (REPO_FOLLOW, "submissions"):
// the reports waiting to go to main are in it, so they are not researched again. This checkout is
// never touched. The clone is made the first time, from this checkout's origin.
import { loadEnvFile } from "node:process";
import { spawn, execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

try { loadEnvFile(new URL("../.env", import.meta.url)); }
catch (error) { if (error.code !== "ENOENT") throw error; }
const checkout = fileURLToPath(new URL("..", import.meta.url));
const env = { ...process.env };
env.TRACKER_URL ||= "https://tracker-api.cannabisfreedom.fyi";
env.RESEARCH_RUNNER ||= "local";
env.REPO_FOLLOW ??= "submissions";
// inside this checkout, so the scripts the worker runs from the clone find its node_modules
env.REPO_PATH = env.WORKER_REPO_PATH || fileURLToPath(new URL("../backend/data/worker-repo", import.meta.url));
env.WORKSPACE_PATH ||= "data/worker-jobs";
if (!env.WORKER_TOKEN) {
  console.error("Set WORKER_TOKEN in .env: the token of this worker in the backend's WORKER_TOKENS.");
  process.exit(1);
}
if (!existsSync(`${env.REPO_PATH}/.git`)) {
  const origin = execFileSync("git", ["remote", "get-url", "origin"], { cwd: checkout }).toString().trim();
  console.log(`Making the worker's clone of ${origin} in ${env.REPO_PATH}…`);
  mkdirSync(dirname(env.REPO_PATH), { recursive: true });
  execFileSync("git", ["clone", "--quiet", "--reference", checkout, "--dissociate", origin, env.REPO_PATH], { stdio: "inherit" });
}
const child = spawn("go", ["run", "./cmd/tracker-worker", ...process.argv.slice(2)], { cwd: fileURLToPath(new URL("../backend/", import.meta.url)), env, stdio: "inherit" });
child.on("error", () => { console.error("Could not start the worker. Check that Go is installed."); process.exitCode = 1; });
child.on("exit", (code, signal) => { process.exitCode = signal ? 0 : code ?? 1; });
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => child.kill(signal));
