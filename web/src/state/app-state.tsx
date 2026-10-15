import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, useRef, useState, type ReactNode } from "react";
import {
  addDownload,
  ApiError,
  listDownloads,
  pauseDownload,
  removeDownload,
  resumeDownload,
  validateConnection,
} from "@/api/client";
import { readSavedConnections, readStoredConnection, removeStoredConnection, saveStoredConnection, writeStoredConnection } from "@/api/connection-storage";
import { streamEvents } from "@/api/sse";
import { appReducer, countsFor, initialState, type LogEntry } from "@/state/app-reducer";
import type { Connection, Download, FilterValue, SavedConnection, ThroughputPoint } from "@/types";

/**
 * A `progress` event carries the full sample (downloaded, total, speed, elapsed)
 * once data is moving, but omits the byte fields on its first frames and keeps
 * omitting them while a download is stalled. This poll reconciles the list, covers
 * that stall and any dropped frames, and surfaces queue changes between events.
 */
const RESYNC_MS = 2_000;
/** Server lines move faster than anyone reads them during a transfer. */
const LOG_WINDOW = 500;

export type AppValue = {
  downloads: Download[];
  visibleDownloads: Download[];
  filter: FilterValue;
  counts: Record<FilterValue, number>;
  throughput: ThroughputPoint[];
  connection: Connection;
  /** Named connections the user kept, seeded from storage after mount. */
  savedConnections: SavedConnection[];
  log: LogEntry[];
  addUrlOpen: boolean;
  connectionOpen: boolean;
  logOpen: boolean;
  /** Sum of the active rows, bytes per second. */
  downloadBps: number;
  /** Megabytes finished plus megabytes moved in the live rows. */
  downloadedMB: number;
  setFilter: (value: FilterValue) => void;
  pause: (id: string) => void;
  resume: (id: string) => void;
  remove: (id: string) => void;
  addUrl: (url: string, savePath: string) => void;
  saveConnection: (name: string, serverUrl: string, token: string) => void;
  removeSavedConnection: (name: string) => void;
  disconnect: () => void;
  setAddUrlOpen: (open: boolean) => void;
  setConnectionOpen: (open: boolean) => void;
  setLogOpen: (open: boolean) => void;
  clearLog: () => void;
};

const AppContext = createContext<AppValue | null>(null);

const messageFor = (cause: unknown, fallback: string): string =>
  cause instanceof ApiError
    ? cause.status === 0
      ? `${fallback}: could not reach the server`
      : `${fallback}: ${cause.message}`
    : fallback;

export function AppProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(appReducer, undefined, initialState);
  // Actions read the live connection without being rebuilt on every stream tick.
  const connectionRef = useRef(state.connection);
  useEffect(() => {
    connectionRef.current = state.connection;
  }, [state.connection]);
  // Same reason: an action names the row it touched in the log.
  const downloadsRef = useRef(state.downloads);
  useEffect(() => {
    downloadsRef.current = state.downloads;
  }, [state.downloads]);
  const nameOf = (id: string) => downloadsRef.current.find(row => row.id === id)?.filename ?? id;

  // Bumped by every explicit Save. The connect effect keys off this as well as the
  // credential pair, so re-saving the values you already have reconnects after a
  // Disconnect — without it that effect still holds the previous pair and no-ops.
  const [saveSeq, setSaveSeq] = useState(0);

  // localStorage is the store; this holds its last read so the tiles re-render.
  const [savedConnections, setSavedConnections] = useState<SavedConnection[]>([]);

  // Stored prefs are read after mount, so the first render is identical on every pass.
  useEffect(() => {
    dispatch({ type: "adoptConnection", connection: readStoredConnection() });
    setSavedConnections(readSavedConnections());
  }, []);

  // Persisted on every connection change; writeStoredConnection saves only the
  // address and token, so a connect or disconnect flip just rewrites the same pair.
  useEffect(() => {
    if (state.connection.serverUrl || state.connection.token) writeStoredConnection(state.connection);
  }, [state.connection]);

  const { connected, serverUrl, token } = state.connection;

  // Validate on each credential pair (and each explicit Save), then hold the
  // stream open until either changes.
  useEffect(() => {
    if (!serverUrl || !token) return;
    const abort = new AbortController();
    let stop: (() => void) | undefined;

    void (async () => {
      const result = await validateConnection(serverUrl, token);
      if (abort.signal.aborted) return;
      if (!result.ok) {
        dispatch({ type: "log", level: "warning", message: result.error });
        dispatch({ type: "disconnect" });
        return;
      }

      dispatch({ type: "log", level: "success", message: `Connected to ${serverUrl}` });
      dispatch({
        type: "adoptConnection",
        connection: { serverUrl, token, connected: true },
      });

      try {
        dispatch({ type: "sync", downloads: await listDownloads(serverUrl, token) });
      } catch {
        // The stream will fill the list in as events arrive.
      }
      if (abort.signal.aborted) return;

      stop = streamEvents(serverUrl, token, {
        onEvent: frame => dispatch({ type: "event", frame }),
        onStatus: status => {
          if (status === "unauthorized") {
            dispatch({ type: "log", level: "warning", message: "The access token was rejected" });
            dispatch({ type: "disconnect" });
          }
        },
      });
    })();

    return () => {
      abort.abort();
      stop?.();
    };
  }, [serverUrl, token, saveSeq]);

  // Periodic resync, so a dropped or missed event cannot leave the list stale.
  useEffect(() => {
    if (!connected) return;
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      void listDownloads(serverUrl, token)
        .then(downloads => dispatch({ type: "sync", downloads }))
        .catch(() => {
          // A single failed poll is not worth a line; the next one tells.
        });
    }, RESYNC_MS);
    return () => window.clearInterval(timer);
  }, [connected, serverUrl, token]);

  // One second of chart movement, paused while the tab is hidden.
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") dispatch({ type: "tick" });
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const guard = useCallback(async (fallback: string, run: () => Promise<void>) => {
    try {
      await run();
    } catch (cause) {
      dispatch({ type: "log", level: "warning", message: messageFor(cause, fallback) });
    }
  }, []);

  const setFilter = useCallback((value: FilterValue) => dispatch({ type: "filter", value }), []);
  const setAddUrlOpen = useCallback((open: boolean) => dispatch({ type: "setAddUrlOpen", open }), []);
  const setConnectionOpen = useCallback((open: boolean) => dispatch({ type: "setConnectionOpen", open }), []);
  const setLogOpen = useCallback((open: boolean) => dispatch({ type: "setLogOpen", open }), []);
  const disconnect = useCallback(() => dispatch({ type: "disconnect" }), []);
  const clearLog = useCallback(() => dispatch({ type: "clearLog" }), []);

  const saveConnection = useCallback((name: string, nextUrl: string, nextToken: string) => {
    // The effect above validates; this only stages the credentials and asks it to run.
    dispatch({ type: "adoptConnection", connection: { serverUrl: nextUrl, token: nextToken, connected: false } });
    setSavedConnections(saveStoredConnection({ name, serverUrl: nextUrl, token: nextToken }));
    dispatch({ type: "log", level: "info", message: `Saved ${name}` });
    setSaveSeq(seq => seq + 1);
  }, []);

  const removeSavedConnection = useCallback((name: string) => {
    setSavedConnections(removeStoredConnection(name));
  }, []);

  const pause = useCallback(
    (id: string) => {
      const { serverUrl: url, token: auth } = connectionRef.current;
      void guard("Could not pause", async () => {
        await pauseDownload(url, auth, id);
        dispatch({ type: "log", level: "info", message: `Paused ${nameOf(id)}` });
        dispatch({ type: "sync", downloads: await listDownloads(url, auth) });
      });
    },
    [guard],
  );

  const resume = useCallback(
    (id: string) => {
      const { serverUrl: url, token: auth } = connectionRef.current;
      void guard("Could not resume", async () => {
        await resumeDownload(url, auth, id);
        dispatch({ type: "log", level: "info", message: `Resumed ${nameOf(id)}` });
        dispatch({ type: "sync", downloads: await listDownloads(url, auth) });
      });
    },
    [guard],
  );

  const remove = useCallback(
    (id: string) => {
      const { serverUrl: url, token: auth } = connectionRef.current;
      void guard("Could not remove", async () => {
        await removeDownload(url, auth, id);
        // The daemon's `removed` event carries the row name, but only if the
        // stream is live, so confirm here too.
        dispatch({ type: "log", level: "warning", message: `Removed ${nameOf(id)}` });
        dispatch({ type: "sync", downloads: await listDownloads(url, auth) });
      });
    },
    [guard],
  );

  const addUrl = useCallback(
    (url: string, savePath: string) => {
      const { serverUrl: base, token: auth } = connectionRef.current;
      void guard("Could not queue the download", async () => {
        const added = await addDownload(base, auth, url, savePath);
        dispatch({ type: "log", level: "info", message: `Queued ${added.filename || url}` });
        dispatch({ type: "sync", downloads: await listDownloads(base, auth) });
      });
    },
    [guard],
  );

  const value = useMemo<AppValue>(() => {
    return {
      downloads: state.downloads,
      visibleDownloads: state.downloads.filter(row =>
        state.filter === "queue"
          ? row.status === "queued"
          : state.filter === "done"
            ? row.status === "done"
            : row.status !== "queued" && row.status !== "done",
      ),
      filter: state.filter,
      counts: countsFor(state.downloads),
      throughput: state.throughput,
      connection: state.connection,
      savedConnections,
      // Trim only what is rendered: the reducer keeps a longer tail for dedupe.
      log: state.log.length > LOG_WINDOW ? state.log.slice(-LOG_WINDOW) : state.log,
      addUrlOpen: state.addUrlOpen,
      connectionOpen: state.connectionOpen,
      logOpen: state.logOpen,
      downloadBps: state.downloadBps,
      downloadedMB: state.downloadedMB,
      setFilter,
      pause,
      resume,
      remove,
      addUrl,
      saveConnection,
      removeSavedConnection,
      disconnect,
      setAddUrlOpen,
      setConnectionOpen,
      setLogOpen,
      clearLog,
    };
  }, [
    state,
    savedConnections,
    setFilter,
    pause,
    resume,
    remove,
    addUrl,
    saveConnection,
    removeSavedConnection,
    disconnect,
    setAddUrlOpen,
    setConnectionOpen,
    setLogOpen,
    clearLog,
  ]);

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

export function useApp(): AppValue {
  const value = useContext(AppContext);
  if (!value) throw new Error("useApp must be used inside AppProvider");
  return value;
}
