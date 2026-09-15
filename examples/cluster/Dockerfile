# The build context is the repository root: the example's go.mod replaces
# github.com/hydroan/gst with ../.., so the framework sources have to be in
# the context. Build from the root:
#
#	docker build -f examples/cluster/Dockerfile -t gst-cluster:dev .
#
# scripts/up.sh does exactly that before it applies the manifests.
FROM golang:1.27 AS build
WORKDIR /src
COPY . .
WORKDIR /src/examples/cluster
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/cluster .

# The sqlite driver the framework links is cgo, so the binary needs libc; a
# slim Debian image carries it and the CA bundle the database client wants.
# tini runs as PID 1 so that a signal sent from inside the pod reaches the
# process — the README's fencing scenario freezes the leader with SIGSTOP,
# which PID 1 itself would ignore — and procps provides the pkill that sends
# it; curl calls the endpoints from inside a pod, for the scenarios and for
# a look at a pod by hand.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl procps tini \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/cluster /app/cluster
COPY examples/cluster/config.ini /app/config.ini
EXPOSE 8080
ENTRYPOINT ["/usr/bin/tini", "--", "/app/cluster"]
