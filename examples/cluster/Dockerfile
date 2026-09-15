# The build context is the repository root: the example's go.mod replaces
# github.com/hydroan/gst with ../.., so the framework sources have to be in
# the context. Build from examples/cluster with
#
#	docker compose build
#
# which passes the right context, or from the root with
#
#	docker build -f examples/cluster/Dockerfile -t gst-cluster:dev .
FROM golang:1.27 AS build
WORKDIR /src
COPY . .
WORKDIR /src/examples/cluster
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/cluster .

# The sqlite driver the framework links is cgo, so the binary needs libc; a
# slim Debian image carries it, the CA bundle the database client wants, and
# curl for the compose health check on /-/readyz.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/cluster /app/cluster
COPY examples/cluster/config.ini /app/config.ini
EXPOSE 8080
ENTRYPOINT ["/app/cluster"]
