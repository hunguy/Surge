import type { ServerDownload, ServerHealth } from "./types.ts";

/**
 * A failed request. `status` is the HTTP status, or 0 when the daemon could not
 * be reached at all, so callers can tell "wrong token" from "wrong port".
 */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

const HEALTH_TIMEOUT_MS = 3_000;
const REQUEST_TIMEOUT_MS = 5_000;

const base = (serverUrl: string) => serverUrl.replace(/\/+$/, "");

const request = async (
  serverUrl: string,
  token: string,
  path: string,
  init: RequestInit = {},
  timeoutMs = REQUEST_TIMEOUT_MS,
): Promise<Response> => {
  let response: Response;
  try {
    response = await fetch(`${base(serverUrl)}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
      // ponytail: fixed timeouts, no retry ladder. The extension retries because a
      // daemon may be starting; the UI just re-syncs on the next tick.
      signal: AbortSignal.timeout(timeoutMs),
    });
  } catch (cause) {
    throw new ApiError(0, cause instanceof Error ? cause.message : "Request failed");
  }

  if (!response.ok) {
    let detail = response.statusText;
    try {
      detail = (await response.text()).trim() || detail;
    } catch {
      // A body that will not read is not worth losing the status over.
    }
    throw new ApiError(response.status, detail);
  }
  return response;
};

/** `/health` is the only route that skips the token. */
export const health = async (serverUrl: string): Promise<ServerHealth> => {
  const response = await request(serverUrl, "", "/health", {}, HEALTH_TIMEOUT_MS);
  return (await response.json()) as ServerHealth;
};

export const listDownloads = async (serverUrl: string, token: string): Promise<ServerDownload[]> => {
  const response = await request(serverUrl, token, "/list");
  const body: unknown = await response.json();
  return Array.isArray(body) ? (body as ServerDownload[]) : [];
};

export const addDownload = async (
  serverUrl: string,
  token: string,
  url: string,
  path: string,
): Promise<{ id: string; filename: string }> => {
  const response = await request(serverUrl, token, "/download", {
    method: "POST",
    // An empty path falls back to the daemon's default output directory.
    body: JSON.stringify({ url, ...(path ? { path } : {}) }),
  });
  const body = (await response.json()) as { id?: string; filename?: string };
  return { id: body.id ?? "", filename: body.filename ?? "" };
};

const idAction = (action: string) => async (serverUrl: string, token: string, id: string) => {
  await request(serverUrl, token, `/${action}?id=${encodeURIComponent(id)}`, { method: "POST" });
};

export const pauseDownload = idAction("pause");
export const resumeDownload = idAction("resume");

/** Queue removal, matching `/delete`. `/purge` would also delete finished files. */
export const removeDownload = idAction("delete");

/**
 * Health first (no token needed), then an authenticated `/list`, so a reachable
 * daemon with a stale token is reported as unreachable rather than connected.
 */
export const validateConnection = async (
  serverUrl: string,
  token: string,
): Promise<{ ok: true } | { ok: false; error: string }> => {
  try {
    await health(serverUrl);
  } catch (cause) {
    const error = cause instanceof ApiError ? cause : new ApiError(0, "Request failed");
    return { ok: false, error: error.status === 0 ? "Could not reach the server" : "Health check failed" };
  }
  try {
    await listDownloads(serverUrl, token);
    return { ok: true };
  } catch (cause) {
    const status = cause instanceof ApiError ? cause.status : 0;
    return { ok: false, error: status === 401 ? "The access token was rejected" : "Could not list downloads" };
  }
};
