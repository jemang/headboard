# The UI is built first and copied into the Go build context, because
# ui/embed.go embeds ui/dist at compile time — the release artifact is one
# binary with no static files to mount.
FROM node:22-alpine AS ui
WORKDIR /src/ui
RUN corepack enable
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY ui/ ./
RUN pnpm build

FROM golang:1.26-alpine AS build
WORKDIR /src
# Dependencies are their own layer so source edits do not re-download the
# Headscale tree, which is large.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY ui/embed.go ./ui/
COPY --from=ui /src/ui/dist ./ui/dist
ARG VERSION=0.0.0-docker
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/headboard ./cmd/headboard

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 headboard \
    && mkdir -p /data && chown headboard /data
COPY --from=build /out/headboard /usr/local/bin/headboard
USER headboard

# Accounts, sessions, the audit log and policy history live here. Mount it, or
# every restart is a first run: a new database means a new owner password.
ENV DATABASE_URL=/data/headboard.db
VOLUME ["/data"]

EXPOSE 3000
ENTRYPOINT ["headboard"]
