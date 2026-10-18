import { test } from "node:test";
import assert from "node:assert/strict";

import { parseEventBlock, drainFrames } from "./sse.ts";

/** Byte-for-byte what cmd/http_api.go writes, plus the CRLF a proxy may rewrite. */
const QUEUED_FRAME = [
  "event: queued",
  'data: {"type":5,"download_id":"queue-1","url":"https://example.com/archive.zip","filename":"archive.zip","dest_path":"/tmp/archive.zip"}',
  "",
  "",
].join("\n");

test("parseEventBlock reads the event name and the JSON payload", () => {
  const frame = parseEventBlock(QUEUED_FRAME.trim());
  assert.equal(frame?.name, "queued");
  assert.equal(frame?.data.download_id, "queue-1");
  assert.equal(frame?.data.type, 5);
});

test("parseEventBlock ignores comments and other fields", () => {
  const frame = parseEventBlock(["id: 7", ": keep-alive", "event: progress", 'data: {"type":1,"downloaded":64}'].join("\n"));
  assert.equal(frame?.name, "progress");
  assert.equal(frame?.data.downloaded, 64);
});

test("parseEventBlock joins split data lines before parsing", () => {
  const frame = parseEventBlock(['event: system', 'data: {"type":11,', 'data: "message":"daemon up"}'].join("\n"));
  assert.equal(frame?.data.message, "daemon up");
});

test("parseEventBlock returns null for a frame with nothing parseable", () => {
  assert.equal(parseEventBlock("event: ping"), null);
  assert.equal(parseEventBlock('data: {"type":1'), null);
  assert.equal(parseEventBlock(""), null);
});

test("parseEventBlock falls back to the numeric Go event type when the name is absent", () => {
  assert.equal(parseEventBlock('data: {"type":2}')?.name, "complete");
  assert.equal(parseEventBlock('data: {"type":11}')?.name, "system");
});

test("drainFrames emits only complete frames and keeps the trailing partial", () => {
  const { frames, rest } = drainFrames(`event: queued\ndata: {"type":5,"download_id":"a"}\n\nevent: progress\ndata: {"type":1,"dow`);
  assert.equal(frames.length, 1);
  assert.equal(frames[0]!.data.download_id, "a");
  assert.equal(rest, 'event: progress\ndata: {"type":1,"dow');
});

test("drainFrames survives a CRLF-rewritten stream", () => {
  const { frames, rest } = drainFrames('event: system\r\ndata: {"type":11,"message":"hi"}\r\n\r\n');
  assert.equal(rest, "");
  assert.equal(frames.length, 1);
  assert.equal(frames[0]!.data.message, "hi");
});

test("drainFrames streams a batch of progress frames in order", () => {
  const batch = [1, 2, 3].map(n => `event: progress\ndata: {"type":1,"download_id":"d${n}"}\n\n`).join("");
  const { frames, rest } = drainFrames(batch);
  assert.deepEqual(
    frames.map(frame => frame.data.download_id),
    ["d1", "d2", "d3"],
  );
  assert.equal(rest, "");
});

test("drainFrames drops a malformed frame without losing the ones around it", () => {
  const { frames } = drainFrames('event: progress\ndata: not json\n\nevent: progress\ndata: {"type":1,"download_id":"ok"}\n\n');
  assert.deepEqual(
    frames.map(frame => frame.data.download_id),
    ["ok"],
  );
});

/** Serves one scripted SSE response per request, recording the request headers. */
const serveSse = async (
  write: (response: import("node:http").ServerResponse, requestCount: number) => void,
) => {
  const { createServer } = await import("node:http");
  const seen: string[] = [];
  const server = createServer((request, response) => {
    seen.push(request.headers.authorization ?? "");
    write(response, seen.length);
  });
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;
  return {
    url: `http://127.0.0.1:${port}`,
    seen,
    close: () => new Promise<void>(resolve => server.close(() => resolve())),
  };
};

test("streamEvents reads frames off the wire and sends the bearer token", async () => {
  const { streamEvents } = await import("./sse.ts");
  const sse = await serveSse(response => {
    response.writeHead(200, { "Content-Type": "text/event-stream" });
    response.write('event: progress\ndata: {"type":1,"download_id":"d1","downloaded":64}\n\n');
  });

  const frames: string[] = [];
  const statuses: string[] = [];
  const stop = streamEvents(sse.url, "test-token", {
    onEvent: frame => frames.push(frame.data.download_id ?? ""),
    onStatus: status => statuses.push(status),
  });

  await new Promise(resolve => setTimeout(resolve, 300));
  stop();
  await sse.close();

  assert.equal(sse.seen[0], "Bearer test-token");
  assert.deepEqual(frames, ["d1"]);
  assert.deepEqual(statuses, ["connecting", "open"]);
});

test("streamEvents gives up on a 401 instead of retrying a token the daemon rejected", async () => {
  const { streamEvents } = await import("./sse.ts");
  const sse = await serveSse(response => {
    response.writeHead(401);
    response.end();
  });

  const statuses: string[] = [];
  const stop = streamEvents(sse.url, "stale-token", {
    onEvent: () => {},
    onStatus: status => statuses.push(status),
  });

  // Long enough to cover several backoff windows had it chosen to retry.
  await new Promise(resolve => setTimeout(resolve, 1_200));
  stop();
  const requests = sse.seen.length;
  await sse.close();

  assert.deepEqual(statuses, ["connecting", "unauthorized"]);
  assert.equal(requests, 1);
});
