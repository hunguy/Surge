import { test } from "node:test";
import assert from "node:assert/strict";

import { applyProgress, countFor, filterMatches, hostFor, toUiDownload, withQueuePositions } from "./map-download.ts";
import type { ServerDownload } from "./types.ts";
import type { Download } from "../types.ts";

const server = (over: Partial<ServerDownload> = {}): ServerDownload => ({
  id: "d1",
  url: "https://viking.pm/files/archive.tar.zst",
  filename: "archive.tar.zst",
  dest_path: "/Volumes/SSD_Ext/Payloads/archive.tar.zst",
  total_size: 1000,
  downloaded: 250,
  progress: 25,
  speed: 500,
  status: "downloading",
  eta: 2,
  connections: 8,
  added_at: 0,
  time_taken: 4000,
  avg_speed: 250,
  ...over,
});

// scheduler.buildStatus is the only producer of these strings.
test("toUiDownload maps every server status the daemon emits", () => {
  const cases: [string, string][] = [
    ["queued", "queued"],
    ["downloading", "active"],
    ["pausing", "active"],
    ["paused", "paused"],
    ["completed", "done"],
    ["error", "error"],
  ];
  for (const [from, to] of cases) {
    assert.equal(toUiDownload(server({ status: from })).status, to);
  }
});

test("toUiDownload keeps an unknown status visible instead of dropping the row", () => {
  assert.equal(toUiDownload(server({ status: "purging" })).status, "queued");
});

test("toUiDownload splits dest_path into folder and takes the host from the url", () => {
  const row = toUiDownload(server());
  assert.equal(row.savePath, "/Volumes/SSD_Ext/Payloads");
  assert.equal(row.host, "viking.pm");
  assert.equal(row.filename, "archive.tar.zst");
});

test("toUiDownload survives a missing dest path and an unparseable url", () => {
  const row = toUiDownload(server({ dest_path: "", url: "not a url" }));
  assert.equal(row.savePath, "");
  assert.equal(row.host, "unknown host");
});

test("toUiDownload clamps progress, and falls back to the daemon's number when bytes are absent", () => {
  // No byte counts yet (a queued row), so the daemon's own percentage is used.
  assert.equal(toUiDownload(server({ total_size: 0, downloaded: 0, progress: 140 })).progress, 100);
  assert.equal(toUiDownload(server({ total_size: 0, downloaded: 0, progress: -5 })).progress, 0);
  // Byte counts win when they are present.
  assert.equal(toUiDownload(server({ total_size: 1000, downloaded: 999, progress: 3 })).progress, 99.9);
  assert.equal(toUiDownload(server({ eta: 0 })).etaSec, null);
  assert.equal(toUiDownload(server({ eta: 9 })).etaSec, 9);
});

test("toUiDownload reads elapsed from time_taken milliseconds", () => {
  assert.equal(toUiDownload(server({ time_taken: 20 * 60 * 1000 + 8000 })).elapsedSec, 1208);
});

test("withQueuePositions numbers queued rows in list order only", () => {
  const rows: Download[] = [
    toUiDownload(server({ id: "a", status: "queued" })),
    toUiDownload(server({ id: "b", status: "downloading" })),
    toUiDownload(server({ id: "c", status: "queued" })),
  ];
  const numbered = withQueuePositions(rows);
  assert.equal(numbered[0]!.queuePosition, 1);
  assert.equal(numbered[1]!.queuePosition, undefined);
  assert.equal(numbered[2]!.queuePosition, 2);
});

test("error rows stay reachable under Active, as the extension does", () => {
  const failed = toUiDownload(server({ status: "error" }));
  assert.equal(filterMatches(failed, "active"), true);
  assert.equal(filterMatches(failed, "queue"), false);
  assert.equal(filterMatches(toUiDownload(server({ status: "completed" })), "done"), true);
  assert.equal(countFor([failed, toUiDownload(server({ status: "paused" }))], "active"), 2);
});

test("applyProgress treats an omitted event field as unchanged, not as zero", () => {
  // Every numeric field on a DownloadEvent is omitempty, so a pause event that
  // carries only `downloaded` must not blank the speed or the byte total.
  const before = toUiDownload(server());
  const after = applyProgress(before, { type: 1, downloaded: 500 });
  assert.equal(after.speedBps, before.speedBps);
  assert.equal(after.totalBytes, before.totalBytes);
  assert.equal(after.progress, 50);
});

test("applyProgress reads elapsed in nanoseconds and recomputes eta from real bytes", () => {
  const after = applyProgress(toUiDownload(server({ progress: 0 })), {
    type: 1,
    downloaded: 500,
    total: 1000,
    speed: 100,
    elapsed: 12_500_000_000,
  });
  assert.equal(after.elapsedSec, 13);
  assert.equal(after.etaSec, 5);
});

test("applyProgress leaves eta alone while the download is not moving", () => {
  const after = applyProgress(toUiDownload(server({ eta: 42 })), { type: 1, speed: 0 });
  assert.equal(after.etaSec, 42);
});

test("hostFor reads a port-qualified host verbatim", () => {
  assert.equal(hostFor("http://127.0.0.1:1700/list"), "127.0.0.1:1700");
});

test("hostFor drops subdomains so a row names the site, not a CDN node", () => {
  assert.equal(hostFor("https://download.real-debrid.com/d/abc/file.mkv"), "real-debrid.com");
  assert.equal(hostFor("https://a.b.c.example.co.uk/x"), "example.co.uk");
  assert.equal(hostFor("http://localhost:8080/f"), "localhost:8080");
  // Two labels where the last is a country: the second level is part of the name.
  assert.equal(hostFor("https://cdn.example.com.au/f"), "example.com.au");
});
