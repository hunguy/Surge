import type { ThroughputPoint } from "./types.ts";

/** Seconds between samples, matching the one second sampling tick. */
export const SAMPLE_SECONDS = 1;
const WINDOW = 30;

const pad = (value: number) => String(value).padStart(2, "0");

/** Local wall-clock label for a sample. */
const clockLabel = (at: number): string => {
  const date = new Date(at);
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
};

const makePoint = (index: number, at: number, downloadMB: number): ThroughputPoint => {
  const label = clockLabel(at);
  return {
    key: `sample-${index}`,
    label,
    axisLabel: index % 6 === 0 ? label.slice(0, 5) : undefined,
    index,
    // The daemon exposes no upload figure, so the series stays at zero.
    values: { download: downloadMB, upload: 0 },
  };
};

/** A full window of zeroes, so the chart has an axis to grow into on first paint. */
export const initialThroughput = (): ThroughputPoint[] => {
  const now = Date.now();
  return Array.from({ length: WINDOW }, (_, index) =>
    makePoint(index, now - (WINDOW - 1 - index) * SAMPLE_SECONDS * 1000, 0),
  );
};

/** Push one sample of the live aggregate speed (bytes/s) and drop the oldest. */
export const advanceThroughput = (
  points: ThroughputPoint[],
  downloadBps: number,
): ThroughputPoint[] => {
  const last = points[points.length - 1];
  const next = makePoint((last?.index ?? -1) + 1, Date.now(), downloadBps / 1_000_000);
  return [...points.slice(1), next];
};

export const peakOf = (points: ThroughputPoint[]): number =>
  points.reduce((peak, point) => Math.max(peak, point.values.download), 0);

export const minOf = (points: ThroughputPoint[]): number =>
  points.length === 0 ? 0 : points.reduce((low, point) => Math.min(low, point.values.download), Infinity);

export const averageOf = (points: ThroughputPoint[]): number =>
  points.length === 0
    ? 0
    : points.reduce((total, point) => total + point.values.download, 0) / points.length;
