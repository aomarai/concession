# Development

## Prerequisites

Go (version in `backend/go.mod`), Docker (for local Postgres), and Node 22 once the frontend lands. The `.devcontainer/` provides all of these.

## Running the backend

```sh
docker compose up -d db
cp backend/.env.example backend/.env
cd backend && go run ./cmd/server
```

With no `DB_DRIVER` set the server uses SQLite (`DB_PATH`, default `concession.db`), so Docker is optional for quick experiments. Schema is created with GORM `AutoMigrate` on startup. AutoMigrate never rewrites existing rows, so any change that needs data fixed up (for example the `movies`/`shows` → `movie`/`show` review type rename) goes in `migrate()` in `cmd/server/main.go` as an idempotent step with a test.

### Configuration

All settings are environment variables (a `backend/.env` file is loaded if present; it is gitignored). See `backend/.env.example` for the full list.

| Variable | Purpose |
|---|---|
| `DB_DRIVER` | `sqlite` (default) or `postgres` |
| `DB_HOST` `DB_PORT` `DB_USER` `DB_PASSWORD` `DB_NAME` | Postgres connection |
| `DB_PATH` | SQLite file (default `concession.db`) |
| `PORT` | Listen port (default `8080`) |
| `COOKIE_SECURE` | Defaults to `true`; set `false` for plain-http local dev |
| `COOKIE_SAME_SITE` | `lax` (default), `strict`, `none` (requires `COOKIE_SECURE=true`) |
| `GOOGLE_CLIENT_ID` `GOOGLE_CLIENT_SECRET` `GOOGLE_REDIRECT_URL` | Google OAuth login |
| `CORS_ALLOWED_ORIGINS` | Comma-separated browser origins (default `http://localhost:5173`) |
| `TMDB_READ_ACCESS_TOKEN` | TMDB v4 "API Read Access Token" (bearer). Without it the catalog endpoints fail with 502 and the server logs a warning |
| `TMDB_BASE_URL` | Override the TMDB API URL (tests/proxies); default `https://api.themoviedb.org/3` |

Get a TMDB token at <https://www.themoviedb.org/settings/api> (use the "API Read Access Token", not the v3 API key). Genres are synced from TMDB at startup when a token is set.

Never commit secrets; keep them in `backend/.env` or your environment.

## Project layout

See [AGENTS.md](../AGENTS.md#layout).

## Testing

```sh
cd backend
go vet ./...
go test ./...
./scripts/coverage.sh        # tests with -race + coverage, enforces the ratchet
go tool cover -html=coverage.out   # browse uncovered lines
```

Conventions:

- Tests use in-memory SQLite via `internal/testutil.NewDB`; never call real external APIs (use `httptest` servers).
- To test database failure paths, use `testutil.FailOn(t, db, "create"|"query"|"update"|"delete"|"row", "<table>")` (`"row"` covers `Scan`/`Row` queries), or `testutil.FailAfter(..., n)` to let the first n calls succeed. Table names are GORM's (e.g. `o_auth_accounts`).
- Tests that run goroutines against the database use `testutil.NewFileDB` (WAL file); shared in-memory SQLite fails concurrent cross-table access with "table is locked".
- Randomness sources are package variables (`randRead`) so entropy failures can be simulated.
- New code should land with tests; the goal is ~100% statement coverage.

### Coverage ratchet

`backend/.coverage-threshold` holds the minimum total coverage. CI runs `scripts/coverage.sh --summary`, which fails the build below that number and prints per-function gaps in the job summary. When coverage improves, raise the threshold in the same PR; never lower it to get a PR green. Coverage status per phase is tracked in the "Test coverage tracker" GitHub issue.

Intentionally uncovered code (kept minimal): `main()` (a one-line `os.Exit(execute(nil))`) and the `Serve` failure branch in `run`, which cannot be triggered once the listener is bound.

## CI

`.github/workflows/ci.yml` runs on every PR and push to `main`: gofmt check, `go vet`, tests with race detector and the coverage gate; plus the frontend lint/build/test job once `frontend/package.json` exists.
