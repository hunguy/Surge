import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

export type LogLevel = "info" | "success" | "warning";

export type LogEntry = {
  id: string;
  /** Epoch ms. Timeline formats the clock time and groups these by day. */
  at: number;
  level: LogLevel;
  message: string;
};

/** Fixed start time for the seed lines, so the first paint matches everywhere. */
const SEED_BASE = Date.UTC(2026, 9, 13, 10, 41, 2);
const at = (seconds: number) => SEED_BASE + seconds * 1000;

/** Seed lines for the mock. Offsets from a fixed clock, so the first paint matches everywhere. */
const SEED: Omit<LogEntry, "id" | "at">[] = [
  { level: "success", message: "Connected to http://127.0.0.1:8181" },
  { level: "info", message: "Resuming 2 downloads from the last session" },
  { level: "info", message: "GET /download/File_name_1.exfat started" },
  { level: "info", message: "Opened 8 connections to Viking.pm" },
  { level: "info", message: "Wrote 64 MB to /Volumes/SSD_Ext/Payloads" },
  { level: "warning", message: "Connection to 1file.com stalled, retrying" },
  { level: "success", message: "Reconnected to 1file.com after 1 attempt" },
  { level: "info", message: "GET /download/File_name_2.mkv started" },
  { level: "info", message: "Wrote 128 MB to /Volumes/SSD_Ext/Video" },
];

const SEED_AT = [0, 0, 1, 3, 10, 98, 99, 148, 196];

type ActivityLogValue = {
  entries: LogEntry[];
  logOpen: boolean;
  setLogOpen: (open: boolean) => void;
  append: (level: LogLevel, message: string) => void;
  clear: () => void;
};

const ActivityLogContext = createContext<ActivityLogValue | null>(null);

export function ActivityLogProvider({ children }: { children: ReactNode }) {
  const [entries, setEntries] = useState<LogEntry[]>(() =>
    SEED.map((entry, index) => ({ ...entry, id: `seed-${index}`, at: at(SEED_AT[index]) })),
  );
  const [logOpen, setLogOpen] = useState(false);

  const append = useCallback((level: LogLevel, message: string) => {
    // The mock clock starts at the seed time and runs forward one second per entry, so lines land on a clock rather than wall time.
    setEntries(current => [
      ...current,
      { id: `live-${current.length}-${message}`, at: at(SEED_AT[SEED_AT.length - 1] + current.length - SEED.length + 1), level, message },
    ]);
  }, []);

  const clear = useCallback(() => setEntries([]), []);

  const value = useMemo<ActivityLogValue>(
    () => ({ entries, logOpen, setLogOpen, append, clear }),
    [entries, logOpen, append, clear],
  );

  return <ActivityLogContext.Provider value={value}>{children}</ActivityLogContext.Provider>;
}

export function useActivityLog(): ActivityLogValue {
  const value = useContext(ActivityLogContext);
  if (!value) throw new Error("useActivityLog must be used inside ActivityLogProvider");
  return value;
}
