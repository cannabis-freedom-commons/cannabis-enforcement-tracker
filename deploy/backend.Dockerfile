# The backend as `docker compose up -d` runs it on a server (compose.yaml, docs/DEPLOY.md). Compose
# builds it from this checkout; nothing is pushed to or pulled from an image registry.
FROM golang:1.27.3-trixie AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /tracker ./cmd/tracker

FROM node:24.15.0-trixie-slim
# git commits and pushes reports (over SSH); ImageMagick with AVIF archives page screenshots.
RUN apt-get update \
 && apt-get install -y --no-install-recommends git openssh-client ca-certificates imagemagick libheif-plugin-aomenc libheif-plugin-dav1d \
 && rm -rf /var/lib/apt/lists/*
# The repository's scripts the backend runs (photo thumbnails) need three packages, at the versions
# of package-lock.json. Node finds them in /node_modules, a parent folder of /repo/scripts.
COPY package-lock.json /tmp/package-lock.json
RUN cd /tmp && npm install --prefix / --no-save --no-package-lock --no-audit --no-fund \
      $(node -p 'const p=require("/tmp/package-lock.json").packages; ["sharp","js-yaml","thumbhash"].map(n=>n+"@"+p["node_modules/"+n].version).join(" ")') \
 && npm cache clean --force && rm /tmp/package-lock.json
COPY --from=build /tracker /usr/local/bin/tracker
# The container runs as the owner of the checkout (their ids from .env), so its commits and data
# stay theirs. ssh finds its config, key and known_hosts through this user's home, where compose
# mounts their ~/.ssh.
ARG TRACKER_UID=1000
ARG TRACKER_GID=1000
RUN userdel node && (getent group "$TRACKER_GID" >/dev/null || groupadd -g "$TRACKER_GID" tracker) \
 && useradd -o -u "$TRACKER_UID" -g "$TRACKER_GID" -d /home/tracker -m -s /bin/sh tracker
USER tracker
ENV HOME=/home/tracker
EXPOSE 8080
ENTRYPOINT ["tracker"]
