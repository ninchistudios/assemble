# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
# Copy the manifests and download first, so the module layer caches and a
# source-only change does not re-fetch the dependency graph. go.* rather than
# naming go.sum, which does not exist until the first module is added.
COPY go.* ./
RUN go mod download
COPY . .
# Defaults to "dev" on purpose: a build that forgets to pass VERSION should say
# so, rather than silently claiming a release number. The real value lives in
# the VERSION file at the repo root.
ARG VERSION=dev
# COMMIT must be passed in: .dockerignore excludes .git, so the build cannot
# discover the revision for itself.
ARG COMMIT=""
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/ninchistudios/assemble/internal/config.Version=${VERSION} \
        -X github.com/ninchistudios/assemble/internal/config.Commit=${COMMIT}" \
      -o /out/assemble .
# An empty directory to seed /data from. Docker initialises a fresh named
# volume from whatever the image has at the mount point, ownership included, so
# this is the only way to hand the volume to an unprivileged user: distroless
# has no shell, so the runtime stage cannot mkdir or chown for itself.
RUN mkdir /out/data

# distroless/static carries CA certificates, which we need to call Google over
# TLS, and nothing else. Runs as an unprivileged user by default.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/assemble /assemble
# 65532 is distroless's nonroot uid. Numeric on purpose: --chown resolves names
# against the target image's /etc/passwd, and the number cannot drift.
COPY --from=build --chown=65532:65532 /out/data /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/assemble"]
