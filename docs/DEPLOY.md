# Running the backend on a server

The backend runs on an Ubuntu server with Docker, from a clone of this repository, at
`https://tracker-api.cannabisfreedom.fyi`. Research runs on workers elsewhere, for example your own
machine (`npm run worker`, see [WORKERS.md](WORKERS.md)).

`docker compose up -d` in the clone builds the backend from that checkout (`compose.yaml`,
`deploy/backend.Dockerfile`) and starts it. No image is pushed to or pulled from a registry, except
the public base images. The container:

- keeps its database and job storage in `backend/data/` of the clone;
- commits new reports into the clone itself, on the `submissions` branch, and pushes them to
  GitHub with a deploy key (your `~/.ssh`, mounted read-only); every 30 minutes they go to `main`
  together (see "Publication" below);
- keeps the clone level with GitHub: it rebases onto `main` before each commit and every five
  minutes (`REPO_SYNC`), so other people's pushes never block its own;
- listens on `127.0.0.1:7706` for the web server in front of it, which serves HTTPS;
- harvests Google News, Bing News and the other feeds, and answers Nostr if configured;
- does no research itself (`RESEARCH_MODE=remote`).

## 1. Prepare the server

Install Docker Engine with the compose plugin (https://docs.docker.com/engine/install/ubuntu/),
and add your user to the `docker` group (`sudo usermod -aG docker $USER`, then log in again).

Point DNS for `tracker-api.cannabisfreedom.fyi` (an A record, and AAAA if the server has IPv6) at
the server.

The backend pushes with a deploy key: an SSH key that may write to this one repository. Make it
on the server, as the user who will own the clone, without a passphrase (the container cannot
type one):

```bash
ssh-keygen -t ed25519 -N "" -C "cet-backend" -f ~/.ssh/cet_deploy
```

Add `~/.ssh/cet_deploy.pub` to the repository on GitHub (Settings → Deploy keys → Add deploy key)
and tick **Allow write access**. Then give it a host name of its own in `~/.ssh/config`, so other
GitHub keys on the machine are left alone:

```
Host github-cet
  HostName github.com
  User git
  IdentityFile ~/.ssh/cet_deploy
  IdentitiesOnly yes
```

Check it once. This also adds github.com to `~/.ssh/known_hosts`, which the container needs:

```bash
ssh -T github-cet
```

The container runs as the same user, with home `/home/tracker`, and `~/.ssh` is mounted there
read-only. So the same config, key and known_hosts work inside it.

## 2. Clone the repository

Clone it through that host name, so pushes use the deploy key:

```bash
sudo mkdir -p /corn/cannabis-freedom-commons && sudo chown $USER: /corn/cannabis-freedom-commons
```

```bash
git clone git@github-cet:cannabis-freedom-commons/cet.git /corn/cannabis-freedom-commons/cet
```

```bash
cd /corn/cannabis-freedom-commons/cet
```

## 3. Settings

Two files in the clone hold the settings. Both are ignored by git.

`.env` is for compose itself: the user the container runs as, so its files and commits are yours
(after changing it, rebuild with `docker compose up -d --build`).

```bash
printf 'TRACKER_UID=%s\nTRACKER_GID=%s\n' "$(id -u)" "$(id -g)" > .env
```

If you run Docker as root but the clone and SSH key are another user's (say `tree`), name that
user and their `.ssh` instead. ssh refuses keys owned by someone else:

```bash
printf 'TRACKER_UID=%s\nTRACKER_GID=%s\nSSH_DIR=/home/tree/.ssh\n' "$(id -u tree)" "$(id -g tree)" > .env
```

`.env.backend` holds the backend's own settings:

```bash
cp deploy/backend.env.example .env.backend
```

```bash
chmod 600 .env.backend
```

Fill in at least:

- `WORKER_TOKENS`: one `name:token` per worker. Make a token with `openssl rand -hex 32`. The worker
  gets the same token as `WORKER_TOKEN`.
- `GIT_AUTHOR_EMAIL` and `GIT_COMMITTER_EMAIL`, and the names if you like: who the report commits
  are by.
- Nostr keys and relays, if the Nostr bot should answer from the server. Move the existing ones
  rather than making new ones, or the bot changes its identity.

Every setting from `backend/config.example.env` can go into `.env.backend`. The paths,
`RESEARCH_MODE`, `REPO_SYNC` and `TRUSTED_PROXY_CIDRS` are set by `compose.yaml`.

## 4. Move the current data (once)

The database and job storage currently live in `backend/data/` on your own machine. Stop the
backend there first (`npm run backend-dev`), so nothing is lost after the copy. Then, in
`backend/` of your local checkout, make a consistent copy of the database:

```bash
go run ./cmd/tracker -backup /tmp/tracker.sqlite
```

The database stores page captures and run logs by their absolute local paths. Rewrite them to the
container's (`/data/jobs/`):

```bash
old="$PWD/data/jobs/"; sqlite3 /tmp/tracker.sqlite "UPDATE source_captures SET artifact_dir=replace(artifact_dir,'$old','/data/jobs/'); UPDATE research_runs SET workspace=replace(workspace,'$old','/data/jobs/');"
```

Copy the database and the job storage (screenshots, logs; several GB) to the server:

```bash
ssh SERVER mkdir -p /corn/cannabis-freedom-commons/cet/backend/data
```

```bash
scp /tmp/tracker.sqlite SERVER:/corn/cannabis-freedom-commons/cet/backend/data/tracker.sqlite
```

```bash
rsync -a --info=progress2 data/jobs/ SERVER:/corn/cannabis-freedom-commons/cet/backend/data/jobs/
```

Replace `SERVER` with the server's SSH address. Without this step the backend starts with an
empty database.

## 5. The web server in front

The backend listens on `127.0.0.1:7706` only. Your web server serves
`https://tracker-api.cannabisfreedom.fyi` and proxies everything to `http://127.0.0.1:7706`, with:

- WebSocket upgrades passed through (live request pages);
- request bodies up to 300 MB (workers upload drafts with photos and page captures);
- `X-Forwarded-For` set to the client's address (rate limits count by it).

## 6. Start it

```bash
docker compose up -d
```

The first start builds the image, which takes a few minutes. Check it:

```bash
docker compose logs -f tracker
```

```bash
curl -s https://tracker-api.cannabisfreedom.fyi/healthz
```

The log should show "research is left to remote workers", and harvesting should start.

## 7. Start the worker on your machine

In your local checkout, add to the root `.env` (the one `backend-dev` reads; your research settings
there stay as they are):

```
TRACKER_URL=https://tracker-api.cannabisfreedom.fyi
WORKER_TOKEN=<the token from WORKER_TOKENS>
```

Then run:

```bash
npm run worker
```

It uses the same research settings as `backend-dev`: `RESEARCH_AGENTS`, the local OpenCode runner
and its saved credentials. It researches against a clone of its own, `backend/data/worker-repo`
(made the first time; `WORKER_REPO_PATH` for another place), which it resets before each job to
the `submissions` branch, so it sees the reports still waiting to go to `main` and does not
research them again. Your checkout is never touched. `npm run worker -- -check-ai` tests the
models and exits.
The status on the request pages says "Research workers: home" once it is connected.

## 8. Point the site at it

The public site turns the submission form on when it is built with the API's address. Set it as a
repository variable; the next Pages build uses it:

```bash
gh variable set PUBLIC_SUBMISSIONS_API_URL --repo cannabis-freedom-commons/cet --body https://tracker-api.cannabisfreedom.fyi
```

## Metrics

`GET /metrics` gives the backend's figures in the Prometheus text format, for Prometheus (or
Grafana Alloy) to scrape and Grafana to chart. Set `METRICS_TOKEN` in `.env.backend` (at least 32
characters, `openssl rand -hex 32`) and restart the backend. Then scrape the backend, on this server
at `127.0.0.1:7706` or from elsewhere through `https://tracker-api.cannabisfreedom.fyi`:

```yaml
scrape_configs:
  - job_name: cet
    scrape_interval: 60s
    scheme: https
    static_configs:
      - targets: [tracker-api.cannabisfreedom.fyi]
    authorization:
      credentials: <METRICS_TOKEN>
```

### VictoriaMetrics on this server

`compose.yaml` has VictoriaMetrics ready as an optional part: it scrapes the backend every minute
over the compose network (`deploy/victoriametrics/scrape.yml`) and keeps two years of data. To start
it with the backend:

1. Set `METRICS_TOKEN` in `.env.backend`, as above.
2. Add the profile to `.env`:
   ```bash
   echo 'COMPOSE_PROFILES=metrics' >> .env
   ```
3. Start it, and restart the backend so it reads the token:
   ```bash
   docker compose up -d --force-recreate
   ```
4. Check that it collects: http://127.0.0.1:8428/targets on the server (through an SSH tunnel,
   `ssh -L 8428:127.0.0.1:8428 SERVER`) should show the `cet` target as up, and
   ```bash
   curl -s 'http://127.0.0.1:8428/api/v1/query?query=tracker_research_ready'
   ```
   should return a value.

In Grafana, add a data source of type **Prometheus** with the URL `http://127.0.0.1:8428` (Grafana on
this server) or the address where Grafana reaches it, then import the dashboard below and choose
that data source. VictoriaMetrics answers the Prometheus API, so the dashboard works as it is.

If you already run VictoriaMetrics (or vmagent) elsewhere, add the job above to its
`-promscrape.config`; it reads the same format.

Workers cannot be scraped behind NAT. Each one sends its figures to the backend with every call,
and the backend lists them as `tracker_worker_*` with a `worker` label.

| Metric | What it shows |
| --- | --- |
| `tracker_jobs{kind,status}` | The queue: research and publish jobs queued, running, done, dead |
| `tracker_research_queue_oldest_seconds` | How long the longest-waiting research job has waited |
| `tracker_cases{status}` | Submitted sources by status (committed, duplicate, failed, …) |
| `tracker_submissions_total{origin}` | Submissions by origin: `web`, `nostr`, `rss` (news discovery) |
| `tracker_harvest_seen_items{provider}` | News items discovery has seen |
| `tracker_research_runs_total{model,status}` | AI runs by model: completed, failed, interrupted |
| `tracker_research_outcomes_total{outcome}` | What completed runs proposed: new_event, additional_source, duplicate, … |
| `tracker_research_tokens_total{model,type}` | Tokens: input, output, reasoning, cache_read, cache_write |
| `tracker_research_cost_usd_total{model}` | Cost at API prices |
| `tracker_research_run_seconds_sum/_count{model}` | Run durations |
| `tracker_reports_committed_total` | Reports committed by the backend |
| `tracker_budget_reserved_cents`, `tracker_budget_daily_limit_cents` | Today's research budget |
| `tracker_research_ready` | 1 while a worker is ready to research |
| `tracker_worker_up{worker}`, `tracker_worker_last_seen_seconds` | Whether each worker is connected |
| `tracker_worker_agent_ready{worker,model}`, `tracker_worker_agent_slots` | Each worker's models and slots |
| `tracker_worker_running_jobs{worker}` | Jobs a worker is doing now |
| `tracker_worker_jobs_total{worker,result}` | Jobs a worker finished: done, failed, interrupted, requeued, lost |
| `tracker_http_requests_total{route,code}`, `tracker_http_request_seconds_*` | API traffic and response times |
| `tracker_database_bytes`, `go_goroutines`, `go_memstats_*` | The backend process |

Counters from the database (`_total` from submissions, runs, commits) count everything since the
start of the tracker. Counters of HTTP requests and worker jobs start again when the backend or
the worker restarts. Grafana's `increase()` and `rate()` handle both.

### Grafana dashboard

`deploy/grafana/cet-dashboard.json` is a ready dashboard with these figures: an overview (research
ready, workers online, queue and oldest wait, reports, cost, failed runs, budget, dead jobs), the
queue and intake, research (outcomes, runs, cost and tokens by model, run times, budget), the
workers (a table of each worker, running jobs against ready slots, finished jobs by result, agents)
and the backend (API traffic, errors, response times, memory, database size).

In Grafana: Dashboards → New → Import, upload the file, and pick your Prometheus data source in
the "Prometheus" selector at the top. Or provision it from that folder like any dashboard file. Its
counters over a range use the selected time range (`$__range`), so "Reports committed" says how
many were committed in the last 7 days when the range is 7 days.

The queries behind it, for panels of your own:

- Reports committed per day: `increase(tracker_reports_committed_total[1d])`
- Submissions per hour by origin: `sum by (origin) (increase(tracker_submissions_total[1h]))`
- Research queue: `tracker_jobs{kind="research",status="queued"}` and `tracker_research_queue_oldest_seconds`
- Run outcomes per day: `sum by (outcome) (increase(tracker_research_outcomes_total[1d]))`
- Failed share of runs: `sum(increase(tracker_research_runs_total{status="failed"}[1d])) / sum(increase(tracker_research_runs_total[1d]))`
- Cost per day by model: `sum by (model) (increase(tracker_research_cost_usd_total[1d]))`
- Average run time: `sum(rate(tracker_research_run_seconds_sum[1h])) / sum(rate(tracker_research_run_seconds_count[1h]))`
- Tokens per hour: `sum by (type) (increase(tracker_research_tokens_total[1h]))`
- Workers busy: `sum(tracker_worker_running_jobs) / sum(tracker_worker_agent_slots * on(worker, model) tracker_worker_agent_ready)`
- Alert when no worker is connected: `max(tracker_worker_up) < 1` for 10 minutes

## Publication

A validated report goes through three statuses on its request page:

1. **Waiting to publish** (`committed`): committed to the `submissions` branch and pushed there.
   Workers research against that branch, so nothing is researched twice meanwhile.
2. **Deploying**: the branch went to `main`. Every 30 minutes, at :00 and :30, the backend rebases
   the branch onto `main` and pushes it as a fast-forward: `main` gets the reports' own commits
   and no merge commit. With no new reports, nothing is pushed.
3. **Published**: each push to `main` builds the site (`.github/workflows/deploy.yml`). Every minute
   the backend reads the live `/version.json`; once the deployed commit contains a report, it is
   published, and Nostr submitters get their reply with the report's link.

`/submissions/` shows how many reports wait for the next batch and when it goes, how many are
deploying with a link to the site's build on GitHub, and the last batch. `PUBLISH_INTERVAL` and
`PUBLISH_OFFSET` in `.env.backend` change the timing.

In the server's clone, only the backend publishes. The `submissions` branch tracks itself on
GitHub, so a plain `git push` or `git pull` there touches only that branch, and a hook the backend
installs refuses pushes to `main` from the clone. If reports reach `main` some other way, nothing
breaks: the next batch does not push them again, and they are published once the site has them. If
the clone is switched to another branch, the backend switches it back. Do not commit on the
`submissions` branch by hand: the backend rebases and force-pushes it.

## Updating

The backend keeps the clone up to date itself (it is on the `submissions` branch, rebased onto
`main` every few minutes). For new backend code, rebuild once the backend has pulled it, or pull
first (`git pull` rebases here, so it never makes a merge commit):

```bash
git pull && docker compose up -d --build
```

A restart leaves research that workers are doing running, and their jobs carry on.

## Backups

All state is in `backend/data/` of the clone. For a consistent copy of the database while the
backend runs (`sudo apt install sqlite3` once):

```bash
sqlite3 backend/data/tracker.sqlite ".backup backend/data/backup-$(date +%F).sqlite"
```
