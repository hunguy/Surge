export type DownloadStatus = "queued" | "active" | "done" | "paused" | "error";

export type Download = {
  id: string;
  filename: string;
  savePath: string;
  host: string;
  status: DownloadStatus;
  /** 0 to 100. */
  progress: number;
  /** Bytes per second, non-zero on moving rows only. */
  speedBps: number;
  elapsedSec: number;
  etaSec: number | null;
  /** 1-based, queued rows only. */
  queuePosition?: number;
  totalBytes?: number;
};

export type Connection = {
  serverUrl: string;
  token: string;
  /** True once the URL and token have been validated against a running daemon. */
  connected: boolean;
};

/** A connection the user named and kept. The name is the identity: saving it again replaces it. */
export type SavedConnection = {
  name: string;
  serverUrl: string;
  token: string;
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
