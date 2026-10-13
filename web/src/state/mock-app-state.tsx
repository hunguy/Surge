import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  type ReactNode,
} from "react";
import { advanceThroughput, seedThroughput } from "@/mock/throughput";
import { countFor, seedDownloads } from "@/mock/downloads";
import {
  DEFAULT_CONNECTION,
  readStoredConnection,
  writeStoredConnection,
} from "@/mock/connection";
import { useActivityLog } from "@/state/activity-log";
import type { FilterValue, MockConnection, MockDownload, ThroughputPoint } from "@/mock/types";

type State = {
  downloads: MockDownload[];
  filter: FilterValue;
  throughput: ThroughputPoint[];
  connection: MockConnection;
  addUrlOpen: boolean;
  connectionOpen: boolean;
  /** Counts up once a second; the wobble table and chart sampling both read it. */
  tick: number;
};

type Action =
  | { type: "filter"; value: FilterValue }
  | { type: "pause"; id: string }
  | { type: "resume"; id: string }
  | { type: "remove"; id: string }
  | { type: "addUrl"; url: string; savePath: string }
  | { type: "saveConnection"; serverUrl: string; token: string }
  | { type: "disconnect" }
  | { type: "setAddUrlOpen"; open: boolean }
  | { type: "setConnectionOpen"; open: boolean }
  | { type: "adoptConnection"; connection: MockConnection }
  | { type: "tick" };

const initialState: State = {
  downloads: seedDownloads,
  filter: "active",
  throughput: seedThroughput(),
  connection: DEFAULT_CONNECTION,
  addUrlOpen: false,
  connectionOpen: false,
  tick: 0,
};

/** Fixed wobble around each row's base speed, so the headline and delta move without Math.random. */
const SPEED_WOBBLE = [0, 0.06, -0.03, 0.1, -0.07, 0.04, -0.02, 0.08, -0.05, 0.02, -0.09, 0.05];

/** Name a queued download after the last path segment of its URL. */
const filenameFor = (url: string): string => {
  try {
    const { pathname } = new URL(url);
    const last = pathname.split("/").filter(Boolean).pop();
    return last ? decodeURIComponent(last) : url;
  } catch {
    return url;
  }
};

const hostFor = (url: string): string => {
  try {
    return new URL(url).host;
  } catch {
    return "unknown host";
  }
};

const nextQueuePosition = (downloads: MockDownload[]): number =>
  downloads.reduce((top, item) => Math.max(top, item.queuePosition ?? 0), 0) + 1;

const reducer = (state: State, action: Action): State => {
  switch (action.type) {
    case "filter":
      return { ...state, filter: action.value };

    case "pause":
      return {
        ...state,
        downloads: state.downloads.map(item =>
          item.id === action.id && item.status === "active"
            ? { ...item, status: "paused", speedBps: 0 }
            : item,
        ),
      };

    case "resume":
      return {
        ...state,
        downloads: state.downloads.map(item =>
          item.id === action.id && item.status === "paused"
            ? { ...item, status: "active", speedBps: item.baseSpeedBps }
            : item,
        ),
      };

    case "remove":
      return { ...state, downloads: state.downloads.filter(item => item.id !== action.id) };

    case "addUrl": {
      const added: MockDownload = {
        id: `added-${nextQueuePosition(state.downloads)}-${state.downloads.length}`,
        filename: filenameFor(action.url),
        savePath: action.savePath || "/mnt/Saved/folder",
        host: hostFor(action.url),
        status: "queued",
        progress: 0,
        speedBps: 0,
        baseSpeedBps: 0,
        elapsedSec: 0,
        etaSec: null,
        queuePosition: nextQueuePosition(state.downloads),
        totalBytes: 120_000_000,
      };
      return { ...state, downloads: [...state.downloads, added] };
    }

    case "saveConnection":
      return {
        ...state,
        connection: {
          serverUrl: action.serverUrl,
          token: action.token,
          connected: Boolean(action.serverUrl && action.token),
        },
      };

    case "disconnect":
      return { ...state, connection: { ...state.connection, connected: false } };

    case "setAddUrlOpen":
      return { ...state, addUrlOpen: action.open };

    case "setConnectionOpen":
      return { ...state, connectionOpen: action.open };

    case "adoptConnection":
      return { ...state, connection: action.connection };

    case "tick":
      return {
        ...state,
        tick: state.tick + 1,
        throughput: advanceThroughput(state.throughput),
        downloads: state.downloads.map(item => {
          if (item.status !== "active" || !item.totalBytes) return item;
          const progress = Math.min(100, item.progress + (item.speedBps / item.totalBytes) * 100);
          if (progress >= 100) {
            return { ...item, progress: 100, status: "done", speedBps: 0, etaSec: 0 };
          }
          const remaining = ((100 - progress) / 100) * item.totalBytes;
          return {
            ...item,
            progress,
            // Fixed wobble, not Math.random, so the mock reads the same on every replay.
            speedBps: Math.round(item.baseSpeedBps * (1 + SPEED_WOBBLE[state.tick % SPEED_WOBBLE.length])),
            elapsedSec: item.elapsedSec + 1,
            etaSec: Math.round(remaining / item.speedBps),
          };
        }),
      };

    default:
      return state;
  }
};

type MockAppValue = {
  downloads: MockDownload[];
  visibleDownloads: MockDownload[];
  filter: FilterValue;
  counts: Record<FilterValue, number>;
  throughput: ThroughputPoint[];
  connection: MockConnection;
  addUrlOpen: boolean;
  connectionOpen: boolean;
  /** Sum of the active rows. */
  downloadBps: number;
  /** Megabytes finished plus megabytes moved in the active rows. */
  downloadedMB: number;
  setFilter: (value: FilterValue) => void;
  pause: (id: string) => void;
  resume: (id: string) => void;
  remove: (id: string) => void;
  addUrl: (url: string, savePath: string) => void;
  saveConnection: (serverUrl: string, token: string) => void;
  disconnect: () => void;
  setAddUrlOpen: (open: boolean) => void;
  setConnectionOpen: (open: boolean) => void;
};

const MockAppContext = createContext<MockAppValue | null>(null);

const matchesFilter = (item: MockDownload, filter: FilterValue): boolean => {
  if (filter === "queue") return item.status === "queued";
  if (filter === "done") return item.status === "done";
  return item.status === "active" || item.status === "paused";
};

export function MockAppProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const { append } = useActivityLog();

  // Stored prefs are read after mount, so the first render is identical on every pass.
  useEffect(() => {
    dispatch({ type: "adoptConnection", connection: readStoredConnection() });
  }, []);

  useEffect(() => {
    writeStoredConnection(state.connection);
  }, [state.connection]);

  // One second of movement, paused whenever the tab is hidden.
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") dispatch({ type: "tick" });
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const setFilter = useCallback((value: FilterValue) => dispatch({ type: "filter", value }), []);
  const pause = useCallback(
    (id: string) => {
      dispatch({ type: "pause", id });
      const item = state.downloads.find(download => download.id === id);
      append("info", `Paused ${item?.filename ?? id}`);
    },
    [state.downloads, append],
  );
  const resume = useCallback(
    (id: string) => {
      dispatch({ type: "resume", id });
      const item = state.downloads.find(download => download.id === id);
      append("info", `Resumed ${item?.filename ?? id}`);
    },
    [state.downloads, append],
  );
  const remove = useCallback(
    (id: string) => {
      const item = state.downloads.find(download => download.id === id);
      dispatch({ type: "remove", id });
      append("warning", `Removed ${item?.filename ?? id}`);
    },
    [state.downloads, append],
  );
  const addUrl = useCallback(
    (url: string, savePath: string) => {
      dispatch({ type: "addUrl", url, savePath });
      append("info", `Queued ${url}`);
    },
    [append],
  );
  const saveConnection = useCallback(
    (serverUrl: string, token: string) => {
      dispatch({ type: "saveConnection", serverUrl, token });
      append(serverUrl && token ? "success" : "warning", `Saved server ${serverUrl || "(none)"}`);
    },
    [append],
  );
  const disconnect = useCallback(() => {
    dispatch({ type: "disconnect" });
    append("warning", "Disconnected from the server");
  }, [append]);
  const setAddUrlOpen = useCallback((open: boolean) => dispatch({ type: "setAddUrlOpen", open }), []);
  const setConnectionOpen = useCallback(
    (open: boolean) => dispatch({ type: "setConnectionOpen", open }),
    [],
  );

  const value = useMemo<MockAppValue>(() => {
    return {
      downloads: state.downloads,
      visibleDownloads: state.downloads.filter(item => matchesFilter(item, state.filter)),
      filter: state.filter,
      counts: {
        queue: countFor(state.downloads, "queue"),
        active: countFor(state.downloads, "active"),
        done: countFor(state.downloads, "done"),
      },
      throughput: state.throughput,
      connection: state.connection,
      addUrlOpen: state.addUrlOpen,
      connectionOpen: state.connectionOpen,
      downloadBps: state.downloads.reduce(
        (total, item) => (item.status === "active" ? total + item.speedBps : total),
        0,
      ),
      downloadedMB: state.downloads.reduce((total, item) => {
        const totalMB = (item.totalBytes ?? 0) / 1_000_000;
        if (item.status === "done") return total + totalMB;
        if (item.status !== "active" && item.status !== "paused") return total;
        return total + (item.progress / 100) * totalMB;
      }, 0),
      setFilter,
      pause,
      resume,
      remove,
      addUrl,
      saveConnection,
      disconnect,
      setAddUrlOpen,
      setConnectionOpen,
    };
  }, [
    state,
    setFilter,
    pause,
    resume,
    remove,
    addUrl,
    saveConnection,
    disconnect,
    setAddUrlOpen,
    setConnectionOpen,
  ]);

  return <MockAppContext.Provider value={value}>{children}</MockAppContext.Provider>;
}

export function useMockApp(): MockAppValue {
  const value = useContext(MockAppContext);
  if (!value) throw new Error("useMockApp must be used inside MockAppProvider");
  return value;
}
