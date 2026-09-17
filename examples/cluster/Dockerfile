# The build context is the repository root: the example's go.mod replaces
# github.com/hydroan/gst with ../.., so the framework sources have to be in
# the context. Build from the root:
#
#	docker build -f examples/cluster/Dockerfile -t gst-cluster:dev .
#
# scripts/up.sh does exactly that before it applies the manifests.
FROM golang:1.27-trixie AS build
WORKDIR /src
COPY . .
WORKDIR /src/examples/cluster
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/cluster .

# The sqlite driver the framework links is cgo, so the binary needs libc: a
# slim image of the build image's Debian release carries it, with the CA
# bundle the database client wants. tini runs as PID 1 so that a signal sent
# from inside the pod reaches the process — the README's scenarios freeze a
# replica with SIGSTOP and kill one with SIGKILL, both of which PID 1 itself
# would ignore — and procps provides the pkill that sends them; curl calls the
# endpoints from inside a pod. The process runs as the unprivileged user the
# manifests name and takes its whole configuration from the environment.
FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl procps tini \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/cluster /app/cluster
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/bin/tini", "--", "/app/cluster"]
