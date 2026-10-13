import styles from "./speed-hero.module.css";
import { useEffect, useRef, useState } from "react";
import { MetricCard } from "@/components/arc/metric-card/metric-card";
import { useMockApp } from "@/state/mock-app-state";

const toMegabytes = (bps: number, decimals: number) => Number((bps / 1_000_000).toFixed(decimals));

/** Metric cards announce nothing on their own, so a live region carries the readings while they move. */
export default function SpeedHero() {
  const { downloadBps, downloadedMB } = useMockApp();
  const value = toMegabytes(downloadBps, 1);
  const total = Number(downloadedMB.toFixed(1));
  const [delta, setDelta] = useState(0);
  const previous = useRef(value);

  useEffect(() => {
    const change = value - previous.current;
    previous.current = value;
    setDelta(Number(change.toFixed(1)));
  }, [value]);

  return (
    <section className={styles.row} aria-label="Transfer summary">
      <MetricCard
        label="Download"
        value={value}
        suffix=" MB/s"
        context="vs last sec"
        change={`${delta >= 0 ? "+" : ""}${delta.toFixed(1)}`}
      />
      <MetricCard label="Downloaded" value={total} suffix=" MB" context="This session" />
      <p className={styles.announce} aria-live="polite">
        {value.toFixed(1)} megabytes per second, {delta >= 0 ? "up" : "down"} {Math.abs(delta).toFixed(1)} from the last second
      </p>
    </section>
  );
}
