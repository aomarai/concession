# HTTP API

Base path: `/api/v1` (except `/healthz`). All responses are JSON. Authenticated routes need the `session_token` cookie obtained by logging in.

## Errors

Every error uses the same shape:

```json
{ "error": { "code": "unauthorized", "message": "Unauthorized" } }
```

Codes in use: `bad_request` (400), `unauthorized` (401), `forbidden` (403), `not_found` (404; also returned for unknown routes), `method_not_allowed` (405), `conflict` (409), `internal_error` (500; also returned if a handler panics), `upstream_error` (502), `unhealthy` (503). Request bodies must be JSON and are limited to 1 MiB.

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

A watchlist holds either movies or shows (`type`). Roles: **owner** (the creator), **editor** (can change items) and **viewer** (read-only); see Collaboration below for how people get those roles. A user with no role on a list gets `404` for every route, so the existence of a private list is never revealed. A role that is too weak for the operation gets `403`.

`privacy` controls who else can read a list: `private` (only the owner and accepted collaborators), `shared` (additionally anyone signed in who has the share link) and `public` (any signed-in user can open it by ID or link, read-only). Public lists are not added to other people's own list of lists.

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

### Collaboration and sharing

The owner invites people by username or e-mail (exact match, e-mail case-insensitive) as **editor** or **viewer**. An invitation grants nothing until the invited user accepts it; pending invitations are visible to the owner and to the invitee only. Looking people up this way tells the owner whether an account exists; that is accepted for v1.

| Method | Path | Who | Description |
|---|---|---|---|
| GET | `/api/v1/watchlists/:id/collaborators` | any role | `{owner, members, pending?}`; `pending` invitations are only included for the owner. People appear as `id`, `display_name`, `avatar_url`, never e-mail |
| POST | `/api/v1/watchlists/:id/collaborators` | owner | Body `{user, role}` with `role` `editor` or `viewer`. `201` with the pending member. `404` with "No user found..." for an unknown user, `409` if already invited or already a collaborator, `400` when inviting yourself |
| PATCH | `/api/v1/watchlists/:id/collaborators/:user_id` | owner | Body `{role}`. `204` |
| DELETE | `/api/v1/watchlists/:id/collaborators/:user_id` | owner, or the user themselves | Removes a collaborator or cancels a pending invitation (an owner can remove anyone; a collaborator can only leave). The owner cannot be removed. Removal is permanent, so the person can be invited again. `204` |
| GET | `/api/v1/me/invites` | any user | `{"invites":[...]}`: invitations waiting for you, with the list's title, the role offered and who invited you |
| POST | `/api/v1/invites/:id/accept` | the invitee | `204` |
| POST | `/api/v1/invites/:id/decline` | the invitee | Deletes the invitation. `204` |
| POST | `/api/v1/watchlists/:id/share-token` | owner | Replaces the share token, which disables every existing link. `{"share_token": "..."}` |
| GET | `/api/v1/shared/:token` | any signed-in user | The list behind a share link, read-only (members keep their own role). `404` while the list is `private` or the token is unknown or rotated |

The share token is only shown to the owner (`share_token` on the list). Sharing a link is two steps: set `privacy` to `shared` (or `public`), then send `/shared/<token>` to a friend.

### Watch progress

Personal tracking, separate from watchlists. `:kind` is `movies` or `shows`; `:tmdb_id` is the TMDB ID.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/me/progress?kind=&status=` | `{"progress":[...]}`, most recently updated first, each with its `movie` or `show`. Both filters are optional |
| GET | `/api/v1/me/progress/:kind/:tmdb_id` | One record; `404` if not tracked |
| PUT | `/api/v1/me/progress/:kind/:tmdb_id` | Create or update. Body `{status, last_season_num?, last_episode_num?}`; `status` is `plan_to_watch`, `watching`, `completed` or `dropped`. Season/episode are for shows only, and an episode needs a season. The title is fetched from TMDB if needed |
| DELETE | `/api/v1/me/progress/:kind/:tmdb_id` | Stop tracking. `204` |

### Reviews and ratings

Ratings are whole numbers from 1 to 10. Reviews are visible to every signed-in user; only the author can change or delete one. Each user can review a title once (a second attempt is `409 conflict`); deleting a review lets them review it again. Reviews expose the author's `id`, `display_name` and `avatar_url` only, never their email. Limits: title 200 characters, text 10,000; both are optional, so a bare rating is fine.

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/movies/:tmdb_id/reviews`, `/api/v1/shows/:tmdb_id/reviews` | Body `{rating, title?, content?}`. The title is fetched from TMDB if needed. `201` with the review |
| GET | `/api/v1/movies/:tmdb_id/reviews`, `/api/v1/shows/:tmdb_id/reviews?page=&per_page=` | `{reviews, summary:{count, average}, page, per_page, total}`, newest first. `average` is rounded to one decimal and is 0 with no reviews. A title nobody has reviewed (or that was never stored) returns an empty page without calling TMDB |
| GET | `/api/v1/me/reviews?page=&per_page=` | Your reviews, newest first, each with its `movie` or `show` |
| GET | `/api/v1/reviews/:id` | One review with its title |
| PATCH | `/api/v1/reviews/:id` | Author only. Any of `rating`, `title`, `content`; omitted fields are unchanged |
| DELETE | `/api/v1/reviews/:id` | Author only. `204` |

Pagination: `page` defaults to 1 and `per_page` to 20 (maximum 100, larger values are clamped); non-numeric, non-positive or absurdly large (`page` over 1,000,000) values are `400`.

## CORS

Origins listed in `CORS_ALLOWED_ORIGINS` may make credentialed requests (cookies). Others get no CORS headers.

---
*Keep this file in sync with `setupRouter` in `backend/cmd/server/main.go`; every new endpoint ships with its entry here.*
