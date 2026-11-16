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
    go build -o /out/cluster . \
    && go install github.com/fullstorydev/grpcurl/cmd/grpcurl@v1.9.3

# websocat is to a WebSocket route what curl is to an endpoint and grpcurl to
# an rpc. Debian carries no package of it, so it is built from its crates.io
# release, without TLS and compression: the scenarios connect to ws://
# addresses inside the cluster, and the binary then needs no library beyond
# the C library.
FROM rust:1.100-slim-trixie AS websocat
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    cargo install --locked --root /out --version 1.14.2 \
        --no-default-features --features signal_handler,unix_stdio websocat

# The sqlite driver the framework links is cgo, so the binary needs libc: a
# slim image of the build image's Debian release carries it, with the CA
# bundle the database client wants. tini runs as PID 1 so that a signal sent
# from inside the pod reaches the process — the README's scenarios freeze a
# replica with SIGSTOP and kill one with SIGKILL, both of which PID 1 itself
# would ignore — and procps provides the pkill that sends them; curl calls the
# endpoints from inside a pod, grpcurl the rpcs and websocat the WebSocket
# routes, naming the replica to call or the address every replica answers on.
# The process runs as the unprivileged user the manifests name and takes its
# whole configuration from the environment.
FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl procps tini \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/cluster /app/cluster
COPY --from=build /go/bin/grpcurl /usr/local/bin/grpcurl
COPY --from=websocat /out/bin/websocat /usr/local/bin/websocat
USER 65532:65532
EXPOSE 8080 8081
ENTRYPOINT ["/usr/bin/tini", "--", "/app/cluster"]
