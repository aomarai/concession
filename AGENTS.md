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
│       ├── catalog/        # Fetch-and-store titles from TMDB (movies, shows, seasons, genres)
│       ├── tmdb/           # TMDB v3 API client (bearer token, TTL cache)
│       ├── config/         # Env config (go-envconfig), OAuth config
│       ├── domain/         # GORM models + cascade delete helpers
│       ├── watchlist/      # Watchlist/item CRUD, ordering and role checks (owner/editor/viewer)
│       ├── progress/       # Per-user watch status and show season/episode progress
│       ├── reviews/        # Ratings (1-10) and reviews; one per user per title; author-only edits
│       ├── svcerr/         # Sentinel errors services return; mapped to HTTP in handlers
│       ├── keyedlock/      # Per-key mutexes to make check-then-write sequences atomic
│       ├── handlers/       # Gin handlers and middleware (auth, CORS, health, errors)
│       ├── logging/        # slog JSON logger + request middleware
│       └── testutil/       # Shared test helpers (in-memory DB, failure injection)
├── docs/                   # API.md (endpoint reference), DEVELOPMENT.md (setup, testing, coverage)
└── frontend/               # Vite + React + TS + Tailwind + React Query (Phase 5)
```

## Conventions

- Routes live under `/api/v1`, registered in `setupRouter` in `cmd/server/main.go`. `/healthz` is outside the prefix.
- Errors: always use `handlers.RespondError(c, status, code, message)` → `{"error":{"code","message"}}`. Services return `svcerr` sentinels (or `catalog.ErrUpstream`); handlers map them with `handlers.RespondServiceError`. Non-members get 404 (not 403) so private lists are not revealed. Access = owner, or an *accepted* collaborator's role, or read-only viewer for anyone on a `public` list (`watchlist.access`); pending invites grant nothing.
- Services take small interfaces (e.g. `watchlist.Catalog`) so tests can fake TMDB; use `keyedlock` for read-then-write sequences and DB constraints for cross-process safety.
- Pass `c.Request.Context()` through to services/DB (`db.WithContext(ctx)`) so cancellation works.
- Use `logging.FromContext(ctx)` for logs; log structured key/values, never secrets.
- Models embed `domain.BaseUUID` (UUID PK, soft delete). Soft deletes do not fire FK cascades, so use the `Delete*Cascade` helpers.
- Collaborators are hard-deleted on removal/decline (unique list+user index), so people can be re-invited; invitations are `Collaborator` rows with status `pending`.
- Reviews are hard-deleted (so the unique user+title index allows re-reviewing); watchlists, items and other models soft-delete.
- Titles: metadata comes from TMDB and both movies and shows are looked up by TMDB ID. Shows also store a nullable, unique `TVDBID` (read from TMDB `external_ids`) as their TVDB external key.
- `WatchlistItem` must reference exactly one of movie/show matching `ItemType` (enforced in `BeforeSave`).
- Login is Google OAuth only (no passwords).
- Tests: `cd backend && go vet ./... && go test ./...`. Use `testutil.NewDB` (in-memory SQLite); never call real external APIs in tests (use `httptest` servers). Test DB failure paths with `testutil.FailOn`.
- **Coverage:** aim for 100% of statements. `backend/scripts/coverage.sh` enforces the ratchet in `backend/.coverage-threshold`; raise it when coverage improves, never lower it. Make error paths testable (inject dependencies / package vars) rather than leaving them uncovered.
- **Docs:** every change that adds or alters an endpoint, config variable, or workflow updates `docs/API.md`, `docs/DEVELOPMENT.md`, `backend/.env.example`, and this file in the same PR.
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
