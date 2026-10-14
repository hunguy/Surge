import styles from "./speed-hero.module.css";
import { useEffect, useRef, useState } from "react";
import { MetricCard } from "@/components/arc/metric-card/metric-card";
import { useApp } from "@/state/app-state";

const toMegabytes = (bps: number, decimals: number) => Number((bps / 1_000_000).toFixed(decimals));
const signed = (delta: number) => `${delta >= 0 ? "+" : ""}${delta.toFixed(1)}`;

/** Metric cards announce nothing on their own, so a live region carries the readings while they move. */
export default function SpeedHero() {
  const { downloadBps, downloadedMB } = useApp();
  const value = toMegabytes(downloadBps, 1);
  const total = Number(downloadedMB.toFixed(1));
  // Each chip is its own card's movement: the rate against the last sample, the session total against its own previous reading.
  const [delta, setDelta] = useState(0);
  const [added, setAdded] = useState(0);
  const previous = useRef(value);
  const previousTotal = useRef(total);

  useEffect(() => {
    setDelta(Number((value - previous.current).toFixed(1)));
    previous.current = value;
  }, [value]);

  useEffect(() => {
    setAdded(Number((total - previousTotal.current).toFixed(1)));
    previousTotal.current = total;
  }, [total]);

  return (
    <section className={styles.row} aria-label="Transfer summary">
      <MetricCard label="Download" value={value} suffix="MB/s" context="vs last sec" change={signed(delta)} />
      <MetricCard label="Downloaded" value={total} suffix="MB" context="This session" change={signed(added)} />
      <p className={styles.announce} aria-live="polite">
        {value.toFixed(1)} megabytes per second, {delta >= 0 ? "up" : "down"} {Math.abs(delta).toFixed(1)} from the last second. {total.toFixed(1)} megabytes downloaded this session.
      </p>
    </section>
  );
}
