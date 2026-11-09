# Backend and research workers

The tracker can run as two programs:

- **The backend** (`backend/cmd/tracker`) runs on the public server. It has the database, the API,
  submissions (web form, Nostr), harvesting (Google News, Bing News, RSS, GDELT), the job queue,
  the research budget and publishing (git commit and push of reports). With
  `RESEARCH_MODE=remote` it needs no AI model, OpenCode, Chromium or ffmpeg.
- **Research workers** (`backend/cmd/tracker-worker`) run anywhere with an internet connection, for
  example at home. A worker opens the submitted pages in Chromium, runs the AI research with
  OpenCode, checks every quotation against the pages, builds and checks the draft and downloads its
  photos. It has its own configuration, including its own research models, and no database.

`RESEARCH_MODE=local` (the default) keeps everything in one process, as before. With `remote`,
research jobs wait in the queue while no worker is connected; the backend never researches itself.
Several workers can work for one backend at once, each with its own models and slots.

## How they talk

The worker always connects to the backend, never the other way round, so it works behind NAT with
no open ports. It sends HTTPS requests to `TRACKER_URL/worker/v1/…` with
`Authorization: Bearer <token>`; the backend lists accepted tokens in `WORKER_TOKENS`
(`name:token,…`). The name appears in the backend's log and on the status page.

| Endpoint | What it does |
| --- | --- |
| `POST /worker/v1/hello` | The worker is there; carries its agents and their health (every 30 s). |
| `POST /worker/v1/claim` | Leases the next research job to the worker for 2 minutes. `204` when there is none. |
| `POST /worker/v1/heartbeat` | Renews the lease (every 30 s). `409` when it was lost: the worker stops the job. |
| `POST /worker/v1/call` | One database step of the job (`op` and its arguments, below). |
| `POST /worker/v1/upload/{kind}` | Files of the job as a gzipped tar: `capture`, `runlog` or `draft`. |
| `POST /worker/v1/finish` | Ends the attempt: done, or its error (retried later, as in one process). |

Every call and upload names its job and lease token, and the backend refuses it (`409`) unless that
lease is current. A call may name only its job's case and that case's research runs, and may not
set a publication status (`publishing`, `committed`, …). The budget for a run is the backend's,
whatever the worker is configured with. Calls that fail while the backend cannot be reached (a
restart, a dropped home connection) are retried for up to three minutes; a backend restart does
not take leases from workers.

## What runs where

The research job (`internal/worker`, `process`) does every database step through the `Backend`
interface (`internal/worker/backend.go`). In one process the store implements it (`worker.Local`);
on a worker, `worker.Client` sends each step to the backend, which runs it with `worker.Local`
(`internal/worker/server.go`). So the same code, with all its checks, runs in both modes.

| Step | Where |
| --- | --- |
| Case statuses and outputs, AI transcript, run accounting, checkpoints, budget | backend, by call |
| Source claims and overlaps, holding back for a related submission, requeueing for review | backend, by call |
| Opening pages, screenshots, archive lookups | worker; each capture uploaded (`capture`) |
| OpenCode research, the check_result tool, quotation checks, photo dedupe, draft build and check | worker; each run's logs uploaded when it ends (`runlog`) |
| Downloading new photos and video posters | worker; sent with the draft |
| Coverage and report-code checks under the publication lock, git commit and push | backend (publish lane) |

The backend keeps what a worker uploads in its own job storage under the same names
(`WORKSPACE_PATH/<case>-<n>/`, `WORKSPACE_PATH/sources/<case>/<capture>/`), so the request page,
raw logs, screenshot archiving and clean-up work as for local research.

### Coverage

A worker keeps its own clone of the reports repository (`REPO_PATH`) and resets it before each job
to the backend's publish branch (`REPO_FOLLOW=submissions`), so research sees the reports published
since and those still waiting to go to `main`. Its draft (`upload/draft`) carries the git
blob each changed report started from. Under its publication lock the backend compares these with
its own `HEAD`: a report changed meanwhile sends the draft back for a review against current
coverage, exactly as when two local jobs race. A new report goes through the usual coverage check.

## Setting it up

1. On the backend: generate a token per worker (`openssl rand -hex 32`), set
   `WORKER_TOKENS=home:<token>` and `RESEARCH_MODE=remote`, set `HARVEST_QUEUE_TARGET` to a number
   (the backend does not know the workers' slots), and restart it. The reverse proxy must pass
   `/worker/` to the backend and allow uploads of up to 256 MB.
2. On the worker machine: clone the reports repository, run `npm install` and
   `npm run browser-install` in it, install OpenCode with its credentials (and ffmpeg for video
   posters).
3. Copy `backend/worker.example.env`, fill in `TRACKER_URL`, `WORKER_TOKEN`, `REPO_PATH` and the
   research settings, check them with `go run ./cmd/tracker-worker -check-ai`, then start
   `go run ./cmd/tracker-worker` (in `backend/`) with that environment.

Stopping a worker (Ctrl-C) returns its jobs to the queue without counting the attempt. An
interrupted OpenCode session is kept on the worker, and the next attempt there continues it.
