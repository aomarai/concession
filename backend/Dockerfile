# syntax=docker/dockerfile:1

# Build. The sqlite driver (used for local dev and tests) needs cgo, so the
# image is built on Debian and runs on Debian rather than a static base.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 10001 --no-create-home concession
COPY --from=build /out/server /usr/local/bin/server
USER concession
ENV PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 \
  CMD curl -fsS "http://localhost:${PORT}/healthz" || exit 1
ENTRYPOINT ["/usr/local/bin/server"]
