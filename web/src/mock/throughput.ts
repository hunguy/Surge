import type { ThroughputPoint } from "./types";

/** Seconds between samples. Fixed, so labels stay deterministic on every render. */
export const SAMPLE_SECONDS = 5;
const BASE_SECONDS = 10 * 3600 + 41 * 60;

/** Clock label for a sample index, as a fixed string (no Date.now, so first paint matches). */
export const clockLabel = (index: number): string => {
  const total = BASE_SECONDS + index * SAMPLE_SECONDS;
  const hours = Math.floor(total / 3600) % 24;
  const minutes = Math.floor(total / 60) % 60;
  const seconds = total % 60;
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
};

/** Download MB/s for 30 samples. Static so the chart has a real shape before anything ticks. */
const DOWNLOAD = [
  38, 47, 61, 86, 118, 152, 178, 196, 208, 214, 206, 188, 164, 141, 122, 108, 116, 134, 158, 176,
  191, 203, 197, 179, 155, 133, 119, 128, 149, 168,
];

const UPLOAD = [
  2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 5, 5, 4, 4, 3, 3, 3, 4, 4, 5, 5, 6, 6, 5, 4, 4, 3, 3, 3, 4,
];

export const makePoint = (index: number, download: number, upload: number): ThroughputPoint => ({
  key: `sample-${index}`,
  label: clockLabel(index),
  axisLabel: index % 6 === 0 ? clockLabel(index).slice(0, 5) : undefined,
  index,
  values: { download, upload },
});

export const seedThroughput = (): ThroughputPoint[] =>
  DOWNLOAD.map((download, index) => makePoint(index, download, UPLOAD[index] ?? 0));

/**
 * Push one new sample and drop the oldest. The walk cycles a fixed step list, so the
 * series moves without Math.random and reads the same on every replay.
 */
const STEP = [4, 9, 13, 6, -3, -11, -16, -8, 2, 11, 17, 12, 5, -6, -14, -19, -9, 1, 8, 15];
export const advanceThroughput = (points: ThroughputPoint[]): ThroughputPoint[] => {
  const last = points[points.length - 1];
  const from = last?.values.download ?? 150;
  const step = STEP[(last?.index ?? 0) % STEP.length];
  const next = Math.min(214, Math.max(38, from + step));
  const upload = Math.min(8, Math.max(2, (last?.values.upload ?? 4) + (step % 3 === 0 ? 1 : step % 3 === 1 ? 0 : -1)));
  return [...points.slice(1), makePoint((last?.index ?? 0) + 1, next, upload)];
};

export const peakOf = (points: ThroughputPoint[]): number =>
  points.reduce((peak, point) => Math.max(peak, point.values.download), 0);

export const minOf = (points: ThroughputPoint[]): number =>
  points.length === 0 ? 0 : points.reduce((low, point) => Math.min(low, point.values.download), Infinity);

export const averageOf = (points: ThroughputPoint[]): number =>
  points.length === 0
    ? 0
    : points.reduce((total, point) => total + point.values.download, 0) / points.length;
