import { applyProgress, countFor, rowFromEvent, toUiDownload, withQueuePositions } from "../api/map-download.ts";
import type { SseFrame } from "../api/sse.ts";
import type { ServerDownload } from "../api/types.ts";
import { advanceThroughput, initialThroughput } from "../throughput.ts";
import type { Connection, Download, FilterValue, ThroughputPoint } from "../types.ts";

export type LogEntry = {
  id: string;
  /** Epoch ms. */
  at: number;
  level: "info" | "success" | "warning";
  message: string;
};

export type AppState = {
  downloads: Download[];
  filter: FilterValue;
  throughput: ThroughputPoint[];
  connection: Connection;
  /** Server lines plus local failures. Never seeded. */
  log: LogEntry[];
  addUrlOpen: boolean;
  connectionOpen: boolean;
  logOpen: boolean;
  /** Bytes per second across the moving rows; the chart samples this once a second. */
  downloadBps: number;
  /** Megabytes finished plus megabytes moved in the live rows. */
  downloadedMB: number;
};

export type AppAction =
  | { type: "filter"; value: FilterValue }
  | { type: "adoptConnection"; connection: Connection }
  | { type: "disconnect" }
  | { type: "sync"; downloads: ServerDownload[] }
  | { type: "event"; frame: SseFrame }
  | { type: "log"; level: LogEntry["level"]; message: string }
  | { type: "clearLog" }
  | { type: "tick" }
  | { type: "setAddUrlOpen"; open: boolean }
  | { type: "setConnectionOpen"; open: boolean }
  | { type: "setLogOpen"; open: boolean };

export const initialState = (): AppState => ({
  downloads: [],
  filter: "active",
  throughput: initialThroughput(),
  connection: { serverUrl: "", token: "", connected: false },
  log: [],
  addUrlOpen: false,
  connectionOpen: false,
  logOpen: false,
  downloadBps: 0,
  downloadedMB: 0,
});

let seq = 0;
/** The mock's clock keyed lines by index; wall time is what a real daemon means. */
const logEntry = (level: LogEntry["level"], message: string, log: LogEntry[]): LogEntry[] => [
  ...log,
  { id: `log-${++seq}`, at: Date.now(), level, message },
];

/** Replace a row in place, keeping it where the list put it. */
const patch = (downloads: Download[], id: string, next: (row: Download) => Download): Download[] =>
  downloads.map(row => (row.id === id ? next(row) : row));

/** Rewrite one row's status, refilling the fields that status implies. */
const restatus = (downloads: Download[], id: string, status: Download["status"]): Download[] =>
  withQueuePositions(
    patch(downloads, id, row => ({
      ...row,
      status,
      speedBps: status === "active" ? row.speedBps : 0,
      etaSec: status === "active" ? row.etaSec : null,
      progress: status === "done" ? 100 : row.progress,
    })),
  );

const liveStats = (downloads: Download[]) => ({
  downloadBps: downloads.reduce(
    (total, row) => (row.status === "active" ? total + row.speedBps : total),
    0,
  ),
  downloadedMB: downloads.reduce((total, row) => {
    const totalMB = (row.totalBytes ?? 0) / 1_000_000;
    if (row.status === "done") return total + totalMB;
    if (row.status !== "active" && row.status !== "paused") return total;
    return total + (row.progress / 100) * totalMB;
  }, 0),
});

const applyEvent = (state: AppState, frame: SseFrame): AppState => {
  const event = frame.data;
  const id = event.download_id;
  const log = (level: LogEntry["level"], message: string) => logEntry(level, message, state.log);

  if (frame.name === "system") {
    if (!event.message) return state;
    // The daemon replays recent system lines on reconnect, so drop a repeat.
    const last = state.log[state.log.length - 1];
    if (last?.message === event.message) return state;
    return { ...state, log: log("info", event.message) };
  }

  if (frame.name === "removed") {
    if (!id) return state;
    const row = state.downloads.find(item => item.id === id);
    return {
      ...state,
      downloads: state.downloads.filter(item => item.id !== id),
      log: log("warning", `Removed ${row?.filename ?? id}`),
    };
  }

  if (!id) return state;

  const exists = state.downloads.some(row => row.id === id);
  let downloads = state.downloads;
  let success: string | null = null;

  switch (frame.name) {
    case "queued":
      if (!exists) {
        const row = rowFromEvent(event);
        if (row) downloads = withQueuePositions([...downloads, { ...row, status: "queued" }]);
      }
      break;

    case "started":
      if (exists) downloads = restatus(downloads, id, "active");
      break;

    case "progress":
      if (!exists) {
        const row = rowFromEvent(event);
        if (row) downloads = [...downloads, row];
      } else {
        downloads = patch(downloads, id, row => applyProgress(row, event));
      }
      break;

    case "paused":
      downloads = restatus(downloads, id, "paused");
      if (event.downloaded !== undefined) {
        downloads = patch(downloads, id, row => applyProgress(row, event));
      }
      break;

    case "resumed":
      downloads = restatus(downloads, id, "active");
      break;

    case "error":
      downloads = restatus(downloads, id, "error");
      break;

    case "complete": {
      downloads = restatus(downloads, id, "done");
      if (event.elapsed !== undefined) {
        downloads = patch(downloads, id, row => applyProgress(row, event));
      }
      success = `Finished ${downloads.find(row => row.id === id)?.filename ?? id}`;
      break;
    }

    default:
      // request / batch_request / batch_progress carry nothing a row can use.
      return state;
  }

  return {
    ...state,
    downloads,
    ...liveStats(downloads),
    log: success ? log("success", success) : state.log,
  };
};

export const appReducer = (state: AppState, action: AppAction): AppState => {
  switch (action.type) {
    case "filter":
      return { ...state, filter: action.value };

    case "adoptConnection":
      return { ...state, connection: action.connection };

    case "disconnect":
      // Keep the address and token so reconnecting is one click, but drop every
      // byte of live data: it belongs to the stream that just ended.
      return {
        ...state,
        connection: { ...state.connection, connected: false },
        downloads: [],
        throughput: initialThroughput(),
        downloadBps: 0,
        downloadedMB: 0,
        log: logEntry("warning", "Disconnected from the server", state.log),
      };

    case "sync": {
      const previous = new Map(state.downloads.map(row => [row.id, row]));
      const downloads = withQueuePositions(
        action.downloads.map(server => {
          const row = toUiDownload(server);
          const before = previous.get(row.id);
          // /list reports time_taken: 0 for a running download, so a resync must
          // not discard the seconds its progress events have accumulated.
          return row.status === "active" && before && row.elapsedSec === 0
            ? { ...row, elapsedSec: before.elapsedSec }
            : row;
        }),
      );
      return { ...state, downloads, ...liveStats(downloads) };
    }

    case "event":
      return applyEvent(state, action.frame);

    case "log":
      return { ...state, log: logEntry(action.level, action.message, state.log) };

    case "clearLog":
      return { ...state, log: [] };

    case "tick": {
      const throughput = advanceThroughput(state.throughput, state.downloadBps);
      return { ...state, throughput };
    }

    case "setAddUrlOpen":
      return { ...state, addUrlOpen: action.open };

    case "setConnectionOpen":
      return { ...state, connectionOpen: action.open };

    case "setLogOpen":
      return { ...state, logOpen: action.open };

    default:
      return state;
  }
};

export const countsFor = (downloads: Download[]): Record<FilterValue, number> => ({
  queue: countFor(downloads, "queue"),
  active: countFor(downloads, "active"),
  done: countFor(downloads, "done"),
});
