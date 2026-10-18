import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type ServerResponse } from "node:http";
import type { TestContext } from "node:test";

import {
  ApiError,
  addDownload,
  health,
  listDownloads,
  pauseDownload,
  removeDownload,
  resumeDownload,
  validateConnection,
} from "./client.ts";

type Seen = { method: string; path: string; query: string; auth: string; body: string };

/** Registers its own teardown, so a failed assertion cannot leak the listener. */
const useServer = async (t: TestContext, handler: (response: ServerResponse, seen: Seen) => void) => {
  const requests: Seen[] = [];
  const node = createServer((request, response) => {
    const seen: Seen = {
      method: request.method ?? "",
      path: request.url?.split("?")[0] ?? "",
      query: request.url?.split("?")[1] ?? "",
      auth: request.headers.authorization ?? "",
      body: "",
    };
    requests.push(seen);
    let raw = "";
    request.on("data", chunk => (raw += chunk));
    request.on("end", () => {
      seen.body = raw;
      handler(response, seen);
    });
  });
  await new Promise<void>(resolve => node.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise<void>(resolve => node.close(() => resolve())));
  const address = node.address();
  const port = typeof address === "object" && address ? address.port : 0;
  return { url: `http://127.0.0.1:${port}`, requests };
};

const json = (response: ServerResponse, body: unknown, status = 200) => {
  response.writeHead(status, { "Content-Type": "application/json" });
  response.end(JSON.stringify(body));
};

test("health reads the unauthenticated status payload", async t => {
  const stub = await useServer(t, response => json(response, { status: "ok", port: 1700, mode: "server" }));
  assert.deepEqual(await health(stub.url), { status: "ok", port: 1700, mode: "server" });
  assert.equal(stub.requests[0]!.auth, "");
});

test("listDownloads sends the bearer token and passes the array through", async t => {
  const stub = await useServer(t, response => json(response, [{ id: "d1", status: "downloading" }]));
  const rows = await listDownloads(stub.url, "tok");
  assert.equal(rows.length, 1);
  assert.equal(rows[0]!.id, "d1");
  assert.equal(stub.requests[0]!.path, "/list");
  assert.equal(stub.requests[0]!.auth, "Bearer tok");
});

test("listDownloads treats a non-array body as an empty list", async t => {
  const stub = await useServer(t, response => json(response, { error: "nope" }));
  assert.deepEqual(await listDownloads(stub.url, "tok"), []);
});

test("a trailing slash on the server url does not double up in the path", async t => {
  const stub = await useServer(t, response => json(response, []));
  await listDownloads(`${stub.url}/`, "tok");
  assert.equal(stub.requests[0]!.path, "/list");
});

test("addDownload posts the url with an optional path", async t => {
  const stub = await useServer(t, response => json(response, { id: "new-1", filename: "a.zip" }));
  assert.deepEqual(await addDownload(stub.url, "tok", "https://ex.com/a.zip", "/mnt/dl"), {
    id: "new-1",
    filename: "a.zip",
  });
  assert.deepEqual(JSON.parse(stub.requests[0]!.body), { url: "https://ex.com/a.zip", path: "/mnt/dl" });

  await addDownload(stub.url, "tok", "https://ex.com/a.zip", "");
  // An empty path must be omitted so the daemon applies its default output dir.
  assert.deepEqual(JSON.parse(stub.requests[1]!.body), { url: "https://ex.com/a.zip" });
});

test("a duplicate add is reported in words, not as the daemon's raw JSON", async t => {
  const stub = await useServer(t, response => {
    // The daemon's real duplicate rejection.
    json(response, { message: "Download rejected: Duplicate download detected (Headless mode)" }, 409);
  });
  await assert.rejects(
    () => addDownload(stub.url, "tok", "https://ex.com/a.zip", ""),
    (error: unknown) =>
      error instanceof ApiError &&
      error.status === 409 &&
      error.message === "That URL is already in the queue",
  );
});

test("pause, resume and remove post the id as a query parameter", async t => {
  const stub = await useServer(t, response => json(response, { status: "ok" }));
  await pauseDownload(stub.url, "tok", "id with spaces");
  await resumeDownload(stub.url, "tok", "d2");
  await removeDownload(stub.url, "tok", "d3");
  assert.deepEqual(
    stub.requests.map(request => `${request.method} ${request.path}?${request.query}`),
    ["POST /pause?id=id%20with%20spaces", "POST /resume?id=d2", "POST /delete?id=d3"],
  );
});

test("a rejected token surfaces as an ApiError carrying 401", async t => {
  const stub = await useServer(t, response => {
    response.writeHead(401, { "Content-Type": "text/plain" });
    response.end("unauthorized");
  });
  await assert.rejects(
    () => listDownloads(stub.url, "stale"),
    (error: unknown) => error instanceof ApiError && error.status === 401,
  );
});

test("an unreachable daemon surfaces as an ApiError with status 0", async () => {
  // Nothing is listening on this port, so the connection is refused.
  await assert.rejects(
    () => listDownloads("http://127.0.0.1:1", "tok"),
    (error: unknown) => error instanceof ApiError && error.status === 0,
  );
});

test("validateConnection reports the token, not the port, when only the token is wrong", async t => {
  const stub = await useServer(t, response => {
    if (response.req.url === "/health") return json(response, { status: "ok", port: 1700 });
    response.writeHead(401);
    response.end("unauthorized");
  });
  assert.deepEqual(await validateConnection(stub.url, "stale"), {
    ok: false,
    error: "The access token was rejected",
  });
});

test("validateConnection passes when health and the authenticated list both answer", async t => {
  const stub = await useServer(t, response => {
    if (response.req.url === "/health") return json(response, { status: "ok", port: 1700 });
    json(response, []);
  });
  assert.deepEqual(await validateConnection(stub.url, "tok"), { ok: true });
});

test("validateConnection blames the address when nothing answers", async () => {
  assert.deepEqual(await validateConnection("http://127.0.0.1:1", "tok"), {
    ok: false,
    error: "Could not reach the server",
  });
});
