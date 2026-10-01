# Concession — Agents Guide

Guidance for AI assistants working on **Concession**: a collaborative movie and TV show watchlist app. Go (Gin) backend, React frontend (planned), PostgreSQL (SQLite for local/tests).

Progress is tracked in GitHub issues (one per phase, see "Roadmap") with a branch per phase (`phase-N-...`).

## Layout

```
concession/
├── .devcontainer/          # Dev container (Go + Node)
├── .github/workflows/      # CI (gofmt, vet, test; frontend build when present)
├── docker-compose.yml      # Local Postgres
├── backend/                # Go module github.com/aomarai/concession
│   ├── cmd/server/main.go  # Entry point: config, DB, router, graceful shutdown
│   └── internal/
│       ├── auth/           # Sessions, cookies, Google user find/create
│       ├── config/         # Env config (go-envconfig), OAuth config
│       ├── domain/         # GORM models + cascade delete helpers
│       ├── handlers/       # Gin handlers and middleware (auth, CORS, health, errors)
│       └── logging/        # slog JSON logger + request middleware
└── frontend/               # Vite + React + TS + Tailwind + React Query (Phase 5)
```

## Conventions

- Routes live under `/api/v1`, registered in `setupRouter` in `cmd/server/main.go`. `/healthz` is outside the prefix.
- Errors: always use `handlers.RespondError(c, status, code, message)` → `{"error":{"code","message"}}`.
- Pass `c.Request.Context()` through to services/DB (`db.WithContext(ctx)`) so cancellation works.
- Use `logging.FromContext(ctx)` for logs; log structured key/values, never secrets.
- Models embed `domain.BaseUUID` (UUID PK, soft delete). Soft deletes do not fire FK cascades, so use the `Delete*Cascade` helpers.
- Titles: movies are keyed by TMDB ID; shows are keyed by TVDB ID (`Show.TVDBID`, with `Show.TMDBID` stored for TMDB lookups). Metadata comes from TMDB; TVDB IDs are read from TMDB `external_ids`.
- `WatchlistItem` must reference exactly one of movie/show matching `ItemType` (enforced in `BeforeSave`).
- Login is Google OAuth only (no passwords).
- Tests: `cd backend && go vet ./... && go test ./...`. Use the in-memory SQLite helpers; never call real external APIs in tests (use `httptest` servers).
- Format with `gofmt`; CI rejects unformatted code.

## Configuration

See `backend/.env.example`. Secrets (Google client secret, TMDB token) live in `backend/.env` (gitignored) or the environment, never in the repo.

## Roadmap

0. Foundations / CI / dev env
1. TMDB integration and catalog
2. Watchlists and watch progress
3. Reviews and ratings
4. Sharing, collaboration, notifications, live updates
5. Frontend
6. Polish and deployment
