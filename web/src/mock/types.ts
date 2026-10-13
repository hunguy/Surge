export type DownloadStatus = "queued" | "active" | "done" | "paused" | "error";

export type MockDownload = {
  id: string;
  filename: string;
  savePath: string;
  host: string;
  status: DownloadStatus;
  /** 0 to 100. */
  progress: number;
  /** Bytes per second, active rows only. Jitters around baseSpeedBps each tick. */
  speedBps: number;
  /** Steady state for the row; the mock wobbles speedBps around it. */
  baseSpeedBps: number;
  elapsedSec: number;
  etaSec: number | null;
  queuePosition?: number;
  totalBytes?: number;
};

export type MockConnection = {
  serverUrl: string;
  token: string;
  /** Mock flag. Save with a non-empty URL and token sets it true. */
  connected: boolean;
};

export type ThroughputPoint = {
  key: string;
  label: string;
  axisLabel?: string;
  /** Sample order, so each new point keeps a unique key as older ones drop off. */
  index: number;
  values: { download: number; upload: number };
};

export type FilterValue = "queue" | "active" | "done";
