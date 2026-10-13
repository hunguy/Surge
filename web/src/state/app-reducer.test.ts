import { test } from "node:test";
import assert from "node:assert/strict";

import { appReducer, initialState, type AppState } from "./app-reducer.ts";
import type { SseFrame } from "../api/sse.ts";
import type { ServerDownload } from "../api/types.ts";

const server = (over: Partial<ServerDownload> = {}): ServerDownload => ({
  id: "d1",
  url: "https://viking.pm/a.tar.zst",
  filename: "a.tar.zst",
  dest_path: "/mnt/dl/a.tar.zst",
  total_size: 1000,
  downloaded: 250,
  progress: 25,
  speed: 400,
  status: "downloading",
  eta: 2,
  connections: 8,
  added_at: 0,
  time_taken: 0,
  avg_speed: 0,
  ...over,
});

const event = (name: string, data: Record<string, unknown>): SseFrame => ({
  name,
  data: data as SseFrame["data"],
});

const withDownloads = (downloads: ServerDownload[]): AppState =>
  appReducer(initialState(), { type: "sync", downloads });

test("initial state is disconnected, empty, and shows no configured server", () => {
  const state = initialState();
  assert.deepEqual(state.downloads, []);
  assert.equal(state.connection.connected, false);
  assert.equal(state.filter, "active");
  assert.equal(state.throughput.length, 30);
  assert.ok(state.throughput.every(point => point.values.download === 0));
});

test("sync adopts the list and drops rows the daemon no longer has, including removed ones", () => {
  let state = withDownloads([server({ id: "a" }), server({ id: "b" })]);
  assert.deepEqual(state.downloads.map(row => row.id), ["a", "b"]);

  state = appReducer(state, { type: "sync", downloads: [server({ id: "b" })] });
  assert.deepEqual(state.downloads.map(row => row.id), ["b"]);
});

test("a queued row carries no progress bar movement and is numbered in list order", () => {
  const state = withDownloads([
    server({ id: "a", status: "queued", downloaded: 0, total_size: 0, progress: 0, speed: 0, eta: 0 }),
    server({ id: "b" }),
    server({ id: "c", status: "queued", downloaded: 0, total_size: 0, progress: 0, speed: 0, eta: 0 }),
  ]);
  assert.equal(state.downloads[0]!.queuePosition, 1);
  assert.equal(state.downloads[1]!.queuePosition, undefined);
  assert.equal(state.downloads[2]!.queuePosition, 2);
  assert.equal(state.downloads[0]!.speedBps, 0);
  assert.equal(state.downloads[0]!.etaSec, null);
});

test("a resync keeps the elapsed a running row accumulated from the stream", () => {
  // /list sends time_taken: 0 for a running download, so a resync must not wipe
  // the seconds the progress events have already accounted for.
  let state = withDownloads([server({ id: "a" })]);
  state = appReducer(state, {
    type: "event",
    frame: event("progress", { type: 1, download_id: "a", elapsed: 12_000_000_000 }),
  });
  assert.equal(state.downloads[0]!.elapsedSec, 12);

  state = appReducer(state, { type: "sync", downloads: [server({ id: "a", time_taken: 0 })] });
  assert.equal(state.downloads[0]!.elapsedSec, 12);
});

test("a resync replaces elapsed with the daemon's total once a row stops running", () => {
  let state = withDownloads([server({ id: "a" })]);
  state = appReducer(state, {
    type: "event",
    frame: event("progress", { type: 1, download_id: "a", elapsed: 99_000_000_000 }),
  });
  // The daemon reports a shorter, authoritative time once the row is paused.
  state = appReducer(state, {
    type: "sync",
    downloads: [server({ id: "a", status: "paused", speed: 0, time_taken: 4_000 })],
  });
  assert.equal(state.downloads[0]!.elapsedSec, 4);
  assert.equal(state.downloads[0]!.status, "paused");
});

test("a progress event patches only the row it names", () => {
  const state = appReducer(withDownloads([server({ id: "a" }), server({ id: "b" })]), {
    type: "event",
    frame: event("progress", { type: 1, download_id: "a", downloaded: 500, speed: 100 }),
  });
  assert.equal(state.downloads[0]!.progress, 50);
  assert.equal(state.downloads[0]!.speedBps, 100);
  assert.equal(state.downloads[1]!.progress, 25);
});

test("a progress event for an unknown id adds the row without dropping the rest", () => {
  const state = appReducer(withDownloads([server({ id: "a" })]), {
    type: "event",
    frame: event("progress", {
      type: 1,
      download_id: "late",
      filename: "late.bin",
      url: "https://ex.com/late.bin",
      downloaded: 10,
      total: 100,
    }),
  });
  assert.deepEqual(state.downloads.map(row => row.id), ["a", "late"]);
  assert.equal(state.downloads[1]!.progress, 10);
});

test("a queued event lands as a queued row, not an active one", () => {
  const state = appReducer(initialState(), {
    type: "event",
    frame: event("queued", { type: 5, download_id: "q1", filename: "q.bin", url: "https://ex.com/q.bin" }),
  });
  assert.equal(state.downloads[0]!.status, "queued");
  assert.equal(state.downloads[0]!.queuePosition, 1);
});

test("paused, resumed, complete and error events move the row between buckets", () => {
  const start = withDownloads([server({ id: "a" })]);
  const at = (frame: SseFrame) => appReducer(start, { type: "event", frame });

  assert.equal(at(event("paused", { type: 3, download_id: "a" })).downloads[0]!.status, "paused");
  assert.equal(at(event("resumed", { type: 4, download_id: "a" })).downloads[0]!.status, "active");
  const done = at(event("complete", { type: 2, download_id: "a", elapsed: 21_000_000_000 }));
  assert.equal(done.downloads[0]!.status, "done");
  assert.equal(done.downloads[0]!.progress, 100);
  assert.equal(done.downloads[0]!.elapsedSec, 21);

  const failed = at(event("error", { type: 7, download_id: "a", error: "connection reset" }));
  assert.equal(failed.downloads[0]!.status, "error");
  assert.equal(failed.downloads[0]!.speedBps, 0);
});

test("a removed event drops the row", () => {
  const state = appReducer(withDownloads([server({ id: "a" }), server({ id: "b" })]), {
    type: "event",
    frame: event("removed", { type: 6, download_id: "a" }),
  });
  assert.deepEqual(state.downloads.map(row => row.id), ["b"]);
});

test("system events append to the activity log, and a reconnect does not duplicate older lines", () => {
  const first = appReducer(initialState(), {
    type: "event",
    frame: event("system", { type: 11, message: "daemon starting" }),
  });
  assert.deepEqual(
    first.log.map(entry => entry.message),
    ["daemon starting"],
  );

  // The daemon replays recent system lines when a stream reconnects.
  const second = appReducer(first, {
    type: "event",
    frame: event("system", { type: 11, message: "daemon starting" }),
  });
  assert.equal(second.log.length, 1);

  const third = appReducer(second, {
    type: "event",
    frame: event("system", { type: 11, message: "another line" }),
  });
  assert.equal(third.log.length, 2);
});

test("tick samples the live aggregate speed into the series and reports it as MB/s", () => {
  const state = withDownloads([server({ id: "a", speed: 1_000_000 }), server({ id: "b", speed: 500_000 })]);
  const next = appReducer(state, { type: "tick" });
  const last = next.throughput[next.throughput.length - 1]!;
  assert.equal(last.values.download, 1.5);
  assert.equal(next.throughput.length, 30);
  assert.equal(next.downloadBps, 1_500_000);
});

test("tick samples zero while every row is paused or queued", () => {
  const state = withDownloads([
    server({ id: "a", status: "paused", speed: 0 }),
    server({ id: "b", status: "queued", speed: 0 }),
  ]);
  const next = appReducer(state, { type: "tick" });
  assert.equal(next.throughput[next.throughput.length - 1]!.values.download, 0);
  assert.equal(next.downloadBps, 0);
});

test("disconnecting clears the live data but keeps the saved address and token", () => {
  const connected = appReducer(withDownloads([server({ id: "a" })]), {
    type: "adoptConnection",
    connection: { serverUrl: "http://127.0.0.1:1700", token: "tok", connected: true },
  });
  const offline = appReducer(connected, { type: "disconnect" });

  assert.deepEqual(offline.downloads, []);
  assert.ok(offline.throughput.every(point => point.values.download === 0));
  assert.equal(offline.connection.connected, false);
  assert.equal(offline.connection.serverUrl, "http://127.0.0.1:1700");
  assert.equal(offline.connection.token, "tok");
});

test("downloadedMB counts finished totals plus the moved part of live rows", () => {
  const state = withDownloads([
    server({ id: "a", total_size: 2_000_000, downloaded: 1_000_000, progress: 50 }),
    server({ id: "b", status: "completed", total_size: 3_000_000, downloaded: 3_000_000, progress: 100 }),
    server({ id: "c", status: "queued", total_size: 9_000_000, downloaded: 0, progress: 0 }),
  ]);
  assert.equal(state.downloadedMB, 4);
});
