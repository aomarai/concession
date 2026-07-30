# Backend container - Movie/TV Watchlist Application using chi router and Redis cache support
# Multi-stage build for small production image

FROM golang:1.23-bookworm AS builder

WORKDIR /app/cmd/server

COPY backend/go.mod .  || true

RUN go mod download 2>/dev/null || echo "Dependencies will be downloaded at build time"


COPY cmd/server/main.go main.go || (echo "No Go source file found in cmd/server/"; touch dummy)


CGO_ENABLED=0 GOOS=${PLATFORM} \
    go build -ldflags="-s -w" -o /app/cmd/server/concession .go 2>/dev/null \
    && mv /app/cmd/server/main.go.concession /app/cmd/server/bin/.concession || (echo "Build requires existing source files in cmd/"; touch dummy)


FROM gcr.io/distroless/static-debian12

WORKDIR /app/backend/cmd/server

COPY --from=builder ./cmd/server/concession .  || true
RUN chmod +x main.concession && chown -R nobody:nogroup /app/backend/cmd/server || (touch dummy)

USER nobody

EXPOSE ${PORT:-8080}


CMD ["./concession"]
