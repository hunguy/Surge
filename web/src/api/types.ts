/** Mirrors `types.DownloadStatus` in internal/types/models.go (snake_case JSON tags). */
export type ServerDownload = {
  id: string;
  url: string;
  filename: string;
  dest_path?: string;
  total_size: number;
  downloaded: number;
  /** 0 to 100, as computed by the daemon. */
  progress: number;
  /** Bytes per second. */
  speed: number;
  /** queued | downloading | pausing | paused | completed | error */
  status: string;
  error?: string;
  /** Seconds remaining. Zero when the daemon has no estimate. */
  eta: number;
  connections: number;
  added_at: number;
  /** Milliseconds. */
  time_taken: number;
  avg_speed: number;
  rate_limit?: number;
  rate_limit_set?: boolean;
};

/**
 * Mirrors `types.DownloadEvent`. Every field in Go is `omitempty`, so an absent
 * number means "not carried by this event" and must never be read as zero.
 * `type` is the numeric `EventType`; `elapsed` is nanoseconds.
 */
export type ServerEvent = {
  type?: number;
  download_id?: string;
  url?: string;
  filename?: string;
  dest_path?: string;
  downloaded?: number;
  total?: number;
  speed?: number;
  connections?: number;
  elapsed?: number;
  avg_speed?: number;
  completed?: boolean;
  error?: string;
  message?: string;
  path?: string;
};

export type ServerHealth = {
  status: string;
  port: number;
  mode?: string;
};
