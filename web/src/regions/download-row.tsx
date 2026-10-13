import styles from "./download-row.module.css";
import Badge, { type BadgeTone } from "@/components/arc/badge/badge";
import Progress from "@/components/arc/progress/progress";
import type { DownloadStatus, Download } from "@/types";

const TONES: Record<DownloadStatus, BadgeTone> = {
  queued: "warning",
  active: "success",
  paused: "neutral",
  done: "success",
  error: "danger",
};

const LABELS: Record<DownloadStatus, string> = {
  queued: "Queued",
  active: "Active",
  paused: "Paused",
  done: "Done",
  error: "Failed",
};

const clock = (total: number): string => {
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = Math.round(total % 60);
  const pad = (value: number) => String(value).padStart(2, "0");
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${pad(minutes)}:${pad(seconds)}`;
};

const megabytes = (bps: number) => `${(bps / 1_000_000).toFixed(1)} MB/s`;

/** Row body. Layout only: every piece of chrome is an Arc primitive. */
export default function DownloadRow({ download }: { download: Download }) {
  const running = download.status === "active";
  const showProgress = running || download.status === "paused";
  // Queued rows have neither an elapsed time nor an estimate, so they skip the times line entirely.
  const remaining =
    download.etaSec === null ? "" : download.etaSec > 0 ? clock(download.etaSec) : "done";
  const elapsed = download.elapsedSec > 0 ? clock(download.elapsedSec) : "";

  return (
    <article className={styles.row}>
      <div className={styles.head}>
        <h3 className={styles.filename} title={download.filename}>
          {download.filename}
        </h3>
        <Badge tone={TONES[download.status]}>{LABELS[download.status]}</Badge>
      </div>

      <div className={styles.line}>
        <span className={styles.path} title={download.savePath}>
          {download.savePath}
        </span>
        <span className={styles.host}>{download.host}</span>
      </div>

      {/* On a running download the speed replaces the progress label; the times line keeps elapsed on the left and remaining on the right for every row. */}
      {showProgress && (
        <Progress
          value={download.progress}
          label={running ? megabytes(download.speedBps) : "Paused"}
          showValue
        />
      )}

      {(elapsed || remaining) && (
        <div className={styles.times}>
          <span>{elapsed}</span>
          <span>{remaining}</span>
        </div>
      )}
    </article>
  );
}
