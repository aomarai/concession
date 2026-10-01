# Concession — Agents Guide

Guidance for AI assistants working on **Concession**: a collaborative movie and TV show watchlist app. Go (Gin) backend, React frontend (Phase 5, in progress), PostgreSQL (SQLite for local/tests).

Progress is tracked in GitHub issues (one per phase, see "Roadmap") with a branch per phase (`phase-N-...`).

## Layout

```
concession/
├── .devcontainer/          # Dev container (Go + Node)
├── .github/workflows/      # CI (gofmt, vet, test; frontend build when present)
├── docker-compose.yml      # Local Postgres; `--profile app` adds the API and frontend containers
├── .env.example            # Variables for docker compose (copy to .env)
├── backend/                # Go module github.com/aomarai/concession (Dockerfile)
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
│       ├── events/         # In-process pub/sub hub behind the live-update (SSE) stream
│       ├── friends/        # Friend requests and the friends list (one Friendship row per user pair)
│       ├── notifications/  # Per-user notifications; other services create them via a Notifier interface
│       ├── userref/        # Resolve a user by username, e-mail or ID
│       ├── paging/         # page/per_page normalization shared by list endpoints
│       ├── svcerr/         # Sentinel errors services return; mapped to HTTP in handlers
│       ├── keyedlock/      # Per-key mutexes to make check-then-write sequences atomic
│       ├── handlers/       # Gin handlers and middleware (auth, CORS, health, errors)
│       ├── logging/        # slog JSON logger + request middleware
│       └── testutil/       # Shared test helpers (in-memory DB, failure injection)
├── docs/                   # API.md (endpoint reference), DEVELOPMENT.md (setup, testing, coverage)
└── frontend/               # Vite + React + TS + Tailwind + React Query (Dockerfile, nginx.conf.template)
    └── src/                # api/ (typed fetch client + endpoints), pages/, components/, lib/, test/
```

## Conventions

- Routes live under `/api/v1`, registered in `setupRouter` in `cmd/server/main.go`. `/healthz` is outside the prefix.
- Errors: always use `handlers.RespondError(c, status, code, message)` → `{"error":{"code","message"}}`. Services return `svcerr` sentinels (or `catalog.ErrUpstream`); handlers map them with `handlers.RespondServiceError`. Non-members get 404 (not 403) so private lists are not revealed. Access = owner, or an *accepted* collaborator's role, or read-only viewer for anyone on a `public` list (`watchlist.access`); pending invites grant nothing.
- Services take small interfaces (e.g. `watchlist.Catalog`) so tests can fake TMDB; use `keyedlock` for read-then-write sequences and DB constraints for cross-process safety.
- Pass `c.Request.Context()` through to services/DB (`db.WithContext(ctx)`) so cancellation works.
- Use `logging.FromContext(ctx)` for logs; log structured key/values, never secrets.
- Models embed `domain.BaseUUID` (UUID PK, soft delete). Soft deletes do not fire FK cascades, so use the `Delete*Cascade` helpers.
- Live updates: services publish tiny change events (what changed, never contents) through a nil-safe `Publisher` after a *successful* write; the SSE handler re-checks access per event and only ends a stream on "not found", not on transient DB errors. Streams must end at shutdown (hub.Close).
- People in API responses are always `domain.PublicUser` (id, display name, avatar), never e-mail. Notifications are best-effort side effects: services call a nil-safe `Notifier` and log failures instead of failing the action.
- Friendships use a canonical user pair (`domain.OrderedPair`) with a unique index; declining/cancelling/unfriending hard-deletes the row.
- Collaborators are hard-deleted on removal/decline (unique list+user index), so people can be re-invited; invitations are `Collaborator` rows with status `pending`.
- Reviews are hard-deleted (so the unique user+title index allows re-reviewing); watchlists, items and other models soft-delete.
- Titles: metadata comes from TMDB and both movies and shows are looked up by TMDB ID. Shows also store a nullable, unique `TVDBID` (read from TMDB `external_ids`) as their TVDB external key.
- `WatchlistItem` must reference exactly one of movie/show matching `ItemType` (enforced in `BeforeSave`).
- Login is Google OAuth only (no passwords).
- Tests: `cd backend && go vet ./... && go test ./...`. Use `testutil.NewDB` (in-memory SQLite); never call real external APIs in tests (use `httptest` servers). Test DB failure paths with `testutil.FailOn`.
- **Coverage:** aim for 100% of statements. `backend/scripts/coverage.sh` enforces the ratchet in `backend/.coverage-threshold`; raise it when coverage improves, never lower it. Make error paths testable (inject dependencies / package vars) rather than leaving them uncovered.
- **Docs:** every change that adds or alters an endpoint, config variable, or workflow updates `docs/API.md`, `docs/DEVELOPMENT.md`, `backend/.env.example`, and this file in the same PR.
- Format with `gofmt`; CI rejects unformatted code.
- **Frontend:** work test-first (Vitest + Testing Library, `vi.mock('../api/endpoints')`; never hit a real backend in tests). `npm test` enforces **100%** statement/branch/function/line coverage (`frontend/vite.config.ts`; only `main.tsx` is excluded). All HTTP goes through `src/api/client.ts` (`/api/v1`, cookie session, `ApiError` from the error envelope). Dev server proxies `/api` to the backend (`BACKEND_URL`, default `http://localhost:8080`).

- **Containers:** the backend image needs cgo (sqlite driver), so it is built and run on Debian. The frontend image is nginx serving the SPA and proxying `/api` (unbuffered, long read timeout for SSE). The `docker` CI job builds and smoke-tests the stack; keep `docker compose --profile app` working.

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
