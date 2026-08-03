# syntax=docker/dockerfile:1.9

# The UI is built first and copied into the Go build context, because
# ui/embed.go embeds ui/dist at compile time — the release artifact is one
# binary with no static files to mount.
#
# Both build stages pin to $BUILDPLATFORM and cross-compile from there. CGO is
# off (modernc.org/sqlite is pure Go), so a linux/arm64 image costs one GOARCH
# flag instead of an emulated toolchain running under QEMU.
FROM --platform=$BUILDPLATFORM node:22.16-alpine AS ui
WORKDIR /src/ui
# corepack takes pnpm's version from package.json's packageManager field, so
# the image cannot drift from what .tool-versions pins.
RUN corepack enable
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY ui/ ./
RUN pnpm build

# Pinned to the patch. Headscale is a compile-time dependency here, not just an
# HTTP peer, and its toolchain requirement is exact — a floating 1.26-alpine
# would silently move underneath a release build.
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS build
WORKDIR /src
# Dependencies are their own layer so source edits do not re-download the
# Headscale tree, which is large.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY ui/embed.go ./ui/
COPY --from=ui /src/ui/dist ./ui/dist

ARG VERSION=0.0.0-docker
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/headboard ./cmd/headboard

FROM alpine:3.22

ARG VERSION=0.0.0-docker
ARG REVISION=unknown
ARG CREATED

# Read by registries and by `docker inspect`. org.opencontainers.image.source
# is the one that matters most: it links the package to the repository on GHCR
# and makes provenance checkable.
LABEL org.opencontainers.image.title="Headboard" \
      org.opencontainers.image.description="A control-plane UI for an existing Headscale: devices, policy, people and keys." \
      org.opencontainers.image.source="https://github.com/jemang/headboard" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

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

# /api/health is unauthenticated and reports Headscale connectivity, so an
# orchestrator learns about a broken API key or an unreachable control plane
# rather than only about a dead process. The address is fixed here because
# HEADBOARD_ADDR is a container-internal detail; override the check if you
# change it.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:3000/api/health || exit 1

ENTRYPOINT ["headboard"]
