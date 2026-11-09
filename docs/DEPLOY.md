# Running the backend on a server

The backend runs on an Ubuntu server with Docker, from a clone of this repository, at
`https://tracker-api.cannabisfreedom.fyi`. Research runs on workers elsewhere, for example your own
machine (`npm run worker`, see [WORKERS.md](WORKERS.md)).

`docker compose up -d` in the clone builds the backend from that checkout (`compose.yaml`,
`deploy/backend.Dockerfile`) and starts it. No image is pushed to or pulled from a registry, except
the public base images. The container:

- keeps its database and job storage in `backend/data/` of the clone;
- commits new reports into the clone itself and pushes them to GitHub with a deploy key
  (your `~/.ssh`, mounted read-only);
- keeps the clone level with GitHub: it pulls before each commit and every five minutes
  (`REPO_SYNC`), so other people's pushes never block its own;
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
and its saved credentials. It fast-forwards this checkout before each job. Set `WORKER_REPO_PATH`
to give it a clone of its own instead. `npm run worker -- -check-ai` tests the models and exits.
The status on the request pages says "Research workers: home" once it is connected.

## 8. Point the site at it

The public site turns the submission form on when it is built with the API's address. Set it as a
repository variable; the next Pages build uses it:

```bash
gh variable set PUBLIC_SUBMISSIONS_API_URL --repo cannabis-freedom-commons/cet --body https://tracker-api.cannabisfreedom.fyi
```

## Updating

The backend pulls new reports itself. For new backend code, rebuild from the updated clone:

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
