# Surge web mock

A playable mock of the Surge download manager UI, built with Vite, React, and
[Arc](https://uiarc.dev) components. It recreates the layout from
`../stitch_mobile_download_manager_ui/` with Arc's design language instead of the
original emerald glow chrome.

Everything is local. There is no Surge HTTP or WebSocket call, no auth, and no
browser extension wiring. Downloads, throughput, and the server connection are
mock state in React that resets on reload (the connection prefs persist to
`localStorage`).

## Run it

```bash
npm install
npm run dev      # http://localhost:5173
npm run build    # type check plus production build
npm run lint     # oxlint
```

## What is interactive

- **Queue / Active / Done** filters the list, with live counts on each tab.
- **Swipe a row** left, or use its More actions menu, to pause, resume, or remove.
- **Add URL** opens a bottom sheet on phones and a dialog from 768px up. An empty
  link shows an inline warning.
- **Options, then Server settings** edits the mock server URL and token. Save with
  both fields filled and the header badge reads Connected; Disconnect flips it to
  Not connected. The pair is stored on this device.
- **Theme switch** in the settings dialog moves between Arc's light and dark
  themes.
- A one second tick advances active progress and shifts the throughput series while
  the tab is visible.

## Layout

```
src/
  main.tsx              entry, loads Arc foundation.css and both fonts
  pages/                the one page container
  regions/              composition modules: header, hero, chart, filter, list, dock, overlays
  state/                MockAppProvider context and reducer
  mock/                 types, seed downloads, throughput series, connection storage
  components/arc/       installed by the shadcn CLI, not hand edited
```

Regions hold layout only. Every button, badge, bar, chart, menu, and overlay is an
Arc component; see `src/regions/` for which ids are used and where.
