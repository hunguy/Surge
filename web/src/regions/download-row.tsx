import styles from "./download-row.module.css";
import { useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import Badge, { type BadgeTone } from "@/components/arc/badge/badge";
import Progress from "@/components/arc/progress/progress";
import { formatBytes } from "@/api/map-download.ts";
import type { DownloadStatus, Download } from "@/types";

/** Travel speed while a long name is moving. A marquee has to be linear, so this is not a motion-token duration. */
const MARQUEE_SPEED = 32;

function Filename({ name }: { name: string }) {
  const boxRef = useRef<HTMLHeadingElement>(null);
  const probeRef = useRef<HTMLSpanElement>(null);
  const [marquee, setMarquee] = useState<{ distance: number; duration: number } | null>(null);

  useLayoutEffect(() => {
    const box = boxRef.current;
    const probe = probeRef.current;
    if (!box || !probe) return;

    let cancelled = false;
    const update = () => {
      if (cancelled) return;
      const textWidth = probe.offsetWidth;
      const root = Number.parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
      // Matches the track gap, which is --space-8 (2rem).
      const gap = root * 2;
      const overflows = textWidth > box.clientWidth + 1;
      const next = overflows ? { distance: textWidth, duration: (textWidth + gap) / MARQUEE_SPEED } : null;
      setMarquee(current => {
        if (next === null) return current === null ? current : null;
        if (current && current.distance === next.distance && Math.abs(current.duration - next.duration) < 0.05) return current;
        return next;
      });
    };

    update();
    const observer = new ResizeObserver(update);
    observer.observe(box);
    observer.observe(probe);
    document.fonts?.ready.then(update).catch(() => {});
    return () => {
      cancelled = true;
      observer.disconnect();
    };
  }, [name]);

  return (
    <h3 ref={boxRef} className={styles.filename} title={name}>
      <span ref={probeRef} className={`${styles.filenameText} ${styles.probe}`} aria-hidden="true">
        {name}
      </span>
      {marquee ? (
        <span
          className={styles.marquee}
          style={
            {
              "--marquee-distance": `${marquee.distance}px`,
              "--marquee-duration": `${marquee.duration}s`,
            } as CSSProperties
          }
        >
          <span className={styles.filenameText}>{name}</span>
          <span className={styles.filenameText} aria-hidden="true">
            {name}
          </span>
        </span>
      ) : (
        <span className={styles.filenameText}>{name}</span>
      )}
    </h3>
  );
}

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
        <Filename name={download.filename} />
        <div className={styles.chips}>
          {/* A queued row has no total until the daemon prechecks it, and there is
              no honest size to print before that, so the chip is simply absent. */}
          {download.totalBytes !== undefined && (
            <Badge tone="neutral">{formatBytes(download.totalBytes)}</Badge>
          )}
          <Badge tone={TONES[download.status]}>{LABELS[download.status]}</Badge>
        </div>
      </div>

      <div className={styles.line}>
        <span className={styles.host}>{download.host}</span>
        <span className={styles.path} title={download.savePath}>
          {download.savePath}
        </span>
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
