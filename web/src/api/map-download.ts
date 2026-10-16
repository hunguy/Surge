import type { Download, DownloadStatus, FilterValue } from "../types.ts";
import type { ServerEvent, ServerDownload } from "./types.ts";

/** scheduler.buildStatus is the only producer of the server's status strings. */
const STATUS: Record<string, DownloadStatus> = {
  queued: "queued",
  downloading: "active",
  pausing: "active",
  paused: "paused",
  completed: "done",
  error: "error",
  // ponytail: "purging" is written only to the store, never to /list. Showing an
  // unrecognised status as queued keeps the row visible; map it here if it ever ships.
  purging: "queued",
};

/**
 * Registrable-domain-ish. A full public-suffix list is a dependency and a data
 * file, which is too much for a label: two labels, or three when the last is a
 * country code and the one before it is short enough to be one (so
 * `example.co.uk` and `example.com.au` hold, while `store.bbc.com` falls to
 * `bbc.com`). A port survives, because it distinguishes a local daemon.
 *
 * ponytail: a public-suffix list is the fix if a two-letter second-level domain
 * like `co.jp` or a private suffix like `github.io` ever needs to come out right.
 */
const domainOf = (host: string): string => {
  // An IPv4 literal is four dot-labels, and the last two of them mean nothing.
  // An IPv6 literal is already bracketed and has no labels to trim.
  if (/^\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?$/.test(host)) return host;
  const labels = host.split(".");
  if (labels.length <= 2) return host;
  const last = labels.at(-1)!;
  const secondLast = labels.at(-2)!;
  const thirdLast = labels.at(-3);
  const country = last.length === 2 && secondLast.length <= 3;
  // Without a third label to spare (`co.uk` alone), there is nothing to trim.
  if (country && thirdLast) return labels.slice(-3).join(".");
  return labels.slice(-2).join(".");
};

export const hostFor = (url: string): string => {
  try {
    return domainOf(new URL(url).host);
  } catch {
    return "unknown host";
  }
};

const folderOf = (path: string): string => {
  if (!path) return "";
  const cut = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return cut > 0 ? path.slice(0, cut) : "";
};

const clamp = (value: number, low: number, high: number) => Math.min(high, Math.max(low, value));

export const toUiDownload = (server: ServerDownload): Download => {
  const status = STATUS[server.status] ?? "queued";
  const total = server.total_size || 0;
  const downloaded = server.downloaded || 0;
  // Prefer the daemon's own numbers: an event-derived row has bytes but no
  // percentage, and a queued row has neither yet.
  const progress =
    total > 0 ? (downloaded / total) * 100 : server.progress > 0 ? server.progress : 0;

  return {
    id: server.id,
    filename: server.filename || server.id,
    savePath: folderOf(server.dest_path ?? ""),
    host: hostFor(server.url),
    status,
    progress: clamp(progress, 0, 100),
    speedBps: status === "active" ? server.speed || 0 : 0,
    // /list carries no start time, so a running row's elapsed comes from its
    // progress events and reads 0 until the first one arrives. Rows the daemon
    // has stopped carry a real total in time_taken (milliseconds).
    elapsedSec: Math.round((server.time_taken || 0) / 1000),
    etaSec: status === "active" && server.eta > 0 ? server.eta : null,
    totalBytes: total > 0 ? total : undefined,
    // Unix seconds from the daemon, or 0 when it does not report one. The reducer
    // falls back to stamping arrival order in that case.
    addedAt: server.added_at > 0 ? server.added_at : 0,
  };
};

export const withQueuePositions = (downloads: Download[]): Download[] => {
  let next = 0;
  return downloads.map(download =>
    download.status === "queued" ? { ...download, queuePosition: ++next } : download,
  );
};

export const filterMatches = (download: Download, filter: FilterValue): boolean => {
  if (filter === "queue") return download.status === "queued";
  if (filter === "done") return download.status === "done";
  // Failed rows stay under Active, matching what the extension surfaces.
  return download.status === "active" || download.status === "paused" || download.status === "error";
};

export const countFor = (downloads: Download[], filter: FilterValue): number =>
  downloads.filter(download => filterMatches(download, filter)).length;

/** The row shape an event implies when no /list sample has been seen yet. */
export const rowFromEvent = (event: ServerEvent): Download | null => {
  if (!event.download_id) return null;
  const bytes = event.downloaded ?? 0;
  const total = event.total ?? 0;
  return {
    id: event.download_id,
    filename: event.filename || event.download_id,
    savePath: folderOf(event.dest_path ?? ""),
    host: hostFor(event.url ?? ""),
    status: "active",
    progress: total > 0 ? clamp((bytes / total) * 100, 0, 100) : 0,
    speedBps: event.speed ?? 0,
    elapsedSec: 0,
    etaSec: null,
    totalBytes: total > 0 ? total : undefined,
    addedAt: 0,
  };
};

/**
 * Fold a `progress` event onto one row. Absent fields are left alone rather than
 * zeroed, because `omitempty` drops them from the wire.
 */
export const applyProgress = (download: Download, event: ServerEvent): Download => {
  const totalBytes = event.total && event.total > 0 ? event.total : download.totalBytes;
  const downloaded = event.downloaded ?? null;
  const speedBps = event.speed ?? download.speedBps;
  const progress =
    downloaded !== null && totalBytes && totalBytes > 0
      ? clamp((downloaded / totalBytes) * 100, 0, 100)
      : download.progress;

  // `elapsed` is time.Duration, so the wire value is nanoseconds.
  const elapsedSec =
    event.elapsed !== undefined ? Math.round(event.elapsed / 1_000_000_000) : download.elapsedSec;

  let etaSec = download.etaSec;
  if (speedBps > 0 && totalBytes && totalBytes > 0) {
    etaSec = Math.max(0, Math.round((totalBytes - (downloaded ?? 0)) / speedBps));
  }

  return { ...download, totalBytes, speedBps, progress, elapsedSec, etaSec };
};
