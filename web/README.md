# Surge web

A browser client for the Surge download manager, built with Vite, React, and
[Arc](https://uiarc.dev) components. It recreates the layout from
`../stitch_mobile_download_manager_ui/` with Arc's design language, and drives it
from a running `surge server`.

## Run it

Start the daemon, then the app:

```bash
# in the repo root
go build -o surge .
./surge server            # binds 127.0.0.1:1700 by default
./surge token             # prints the auth token, or pass --token
```

```bash
# in web/
npm install
npm run dev      # http://localhost:5173
npm run build    # type check plus production build
npm run test     # node:test, no test dependencies
npm run lint     # oxlint
```

Open the app, then **Options → Server settings** and enter the server URL
(`http://127.0.0.1:1700`) and the token. Both are saved in this browser and
validated on save; the header badge reads Connected once the daemon answers.

## What is interactive

- **Queue / Active / Done** filters the list, with live counts on each tab.
- **Swipe a row** left to reveal Pause, Resume, and Remove. (The row's More
  actions menu is a known Arc bug: `menuItems` is never populated in
  `components/arc/swipe-actions/swipe-actions.tsx`, so the menu never renders.)
- **Add URL** opens a bottom sheet on phones and a dialog from 768px up, and
  posts to `/download` with the optional destination path.
- **The status pill** opens the activity log: server `system` lines plus local
  connection and action results. Clear is local only.
- **Theme switch** in the settings dialog moves between Arc's light and dark
  themes and is stored on the device.

## How it talks to the server

| UI need | Endpoint |
|---------|----------|
| Connect | `GET /health`, then an authenticated `GET /list` |
| Download list | `GET /list`, polled every 2s |
| Live updates and activity | `GET /events` (SSE) |
| Add URL | `POST /download` `{ url, path }` |
| Pause / Resume | `POST /pause?id=`, `POST /resume?id=` |
| Remove | `POST /delete?id=` (queue removal; not `/purge`) |

`EventSource` cannot send an `Authorization` header, so `src/api/sse.ts` reads
the stream off a `fetch` `ReadableStream` instead, with backoff and a give-up on
401.

Two behaviours worth knowing:

- A `progress` event carries the full sample once a download is moving, but omits
  `downloaded`, `speed` and `total` on its first frames and keeps omitting them
  while the download is stalled. The `/list` poll every 2s reconciles rows and
  covers that gap; the extension's 60s would leave a stalled row looking frozen.
- Every field on a `DownloadEvent` is `omitempty`, so an absent number means
  "not carried by this event" and is left alone, never read as zero. That is what
  makes the stalled case above work: a frame with no `speed` does not zero the row.

## Layout

```
src/
  main.tsx              entry, loads Arc foundation.css and both fonts
  pages/                the one page container
  regions/              composition modules: header, hero, chart, filter, list, dock, overlays
  state/                AppProvider context, plus the pure reducer it drives
  api/                  HTTP client, SSE reader, server types, download mappers
  types.ts              the shapes the UI renders
  throughput.ts         the chart's sample series
  components/arc/       installed by the shadcn CLI, not hand edited
```

Regions hold layout only. Every button, badge, bar, chart, menu, and overlay is an
Arc component; see `src/regions/` for which ids are used and where.

The reducer in `state/app-reducer.ts` is pure and takes no React dependency, so
the mapping from server payloads to rows is unit-tested directly rather than
through the DOM. Run `npm run test`.
