# HTTP API

Base path: `/api/v1` (except `/healthz`). All responses are JSON. Authenticated routes need the `session_token` cookie obtained by logging in.

## Errors

Every error uses the same shape:

```json
{ "error": { "code": "unauthorized", "message": "Unauthorized" } }
```

Codes in use: `bad_request` (400), `unauthorized` (401), `forbidden` (403), `not_found` (404), `conflict` (409), `internal_error` (500), `upstream_error` (502), `unhealthy` (503). Request bodies must be JSON and are limited to 1 MiB.

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/healthz` | no | Liveness + database ping. `200 {"status":"ok"}` or `503` |
| GET | `/api/v1/auth/google/login` | no | Starts Google OAuth; sets `oauth_state` cookie and redirects to Google |
| GET | `/api/v1/auth/google/callback` | no | Completes login; sets `session_token` cookie and redirects to `/` |
| POST | `/api/v1/auth/logout` | no | Revokes the session and clears the cookie |
| GET | `/api/v1/me` | yes | The current user |
| GET | `/api/v1/search?q=&page=` | yes | Search movies and TV shows on TMDB (people are filtered out). Empty `q` returns no results. `page` ≥ 1 |
| GET | `/api/v1/movies/:tmdb_id` | yes | A movie by TMDB ID, with genres and top-billed actors |
| GET | `/api/v1/shows/:tmdb_id` | yes | A show by TMDB ID, with genres and season summaries |
| GET | `/api/v1/shows/:tmdb_id/seasons/:season` | yes | One season of a show with its episodes (`season` 0 = specials) |

### Catalog behavior

- Titles are fetched from TMDB on first request and stored locally; later requests are served from the database. A stored title older than 24 hours is refreshed, and if TMDB is unreachable the stored copy is served instead.
- The returned `id` is the internal ID used by watchlists and reviews; `tmdb_id` is the TMDB ID used in these URLs.
- Both movies and shows are addressed by TMDB ID in URLs (`tmdb_id`). Shows also carry a `tvdb_id`, read from TMDB's external IDs and stored as a unique, optional external key; it is `null` for the few shows TMDB has no TVDB ID for.
- Refreshing a show or season also removes seasons and episodes TMDB no longer lists (an empty TMDB answer removes nothing). A season is served from the stored copy if TMDB is unreachable.
- Concurrent requests for the same title are serialized per process, so a title is fetched and stored once.
- TMDB lookup failures: `404 not_found` when TMDB has no such title, `502 upstream_error` for any other TMDB problem.

### Watchlists

A watchlist holds either movies or shows (`type`). Roles: **owner** (the creator), **editor** (can change items) and **viewer** (read-only); collaborators are added in a later phase. A user with no role on a list gets `404` for every route, so the existence of a private list is never revealed. A role that is too weak for the operation gets `403`.

| Method | Path | Who | Description |
|---|---|---|---|
| POST | `/api/v1/watchlists` | any user | Create. Body `{title, description?, privacy?, type}`; `privacy` is `private` (default), `shared` or `public`; `type` is `movie` or `show`. `201` with the list |
| GET | `/api/v1/watchlists` | any user | `{"watchlists":[...]}`: lists you own or collaborate on, newest first, each with your `role` and `item_count` |
| GET | `/api/v1/watchlists/:id` | any role | The list with its `items` in order, each with the full `movie` or `show`. `share_token` is only included for the owner |
| PATCH | `/api/v1/watchlists/:id` | owner | Body with any of `title`, `description`, `privacy` |
| DELETE | `/api/v1/watchlists/:id` | owner | Deletes the list with its items and collaborators. `204` |
| POST | `/api/v1/watchlists/:id/items` | owner, editor | Body `{tmdb_id, notes?}`. The title is fetched from TMDB if needed and appended. `201` with the item; `409 conflict` if already on the list; `404` if TMDB has no such title |
| PATCH | `/api/v1/watchlists/:id/items/:item_id` | owner, editor | Body `{notes}` |
| DELETE | `/api/v1/watchlists/:id/items/:item_id` | owner, editor | Removes the item and closes the gap in positions. `204` |
| PUT | `/api/v1/watchlists/:id/items/order` | owner, editor | Body `{item_ids:[...]}` listing **every** item exactly once in the new order, else `400`. `204` |

Limits: title 200 characters, description and notes 2000. New users automatically get the lists "Movies to watch" and "Shows to watch".

### Watch progress

Personal tracking, separate from watchlists. `:kind` is `movies` or `shows`; `:tmdb_id` is the TMDB ID.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/me/progress?kind=&status=` | `{"progress":[...]}`, most recently updated first, each with its `movie` or `show`. Both filters are optional |
| GET | `/api/v1/me/progress/:kind/:tmdb_id` | One record; `404` if not tracked |
| PUT | `/api/v1/me/progress/:kind/:tmdb_id` | Create or update. Body `{status, last_season_num?, last_episode_num?}`; `status` is `plan_to_watch`, `watching`, `completed` or `dropped`. Season/episode are for shows only, and an episode needs a season. The title is fetched from TMDB if needed |
| DELETE | `/api/v1/me/progress/:kind/:tmdb_id` | Stop tracking. `204` |

## CORS

Origins listed in `CORS_ALLOWED_ORIGINS` may make credentialed requests (cookies). Others get no CORS headers.

---
*Keep this file in sync with `setupRouter` in `backend/cmd/server/main.go`; every new endpoint ships with its entry here.*
