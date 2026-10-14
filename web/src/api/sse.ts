import type { ServerEvent } from "./types.ts";

export type SseFrame = { name: string; data: ServerEvent };

/** `EventType` iota order in internal/types/events.go, for frames that omit the name. */
const EVENT_NAMES = [
  "started",
  "progress",
  "complete",
  "paused",
  "resumed",
  "queued",
  "removed",
  "error",
  "request",
  "batch_request",
  "batch_progress",
  "system",
];

/** One `\n\n`-terminated block off the wire, as `event:` + `data:` lines. */
export const parseEventBlock = (block: string): SseFrame | null => {
  if (!block.trim()) return null;

  let name = "";
  const data: string[] = [];
  for (const raw of block.split("\n")) {
    const line = raw.replace(/\r$/, "");
    if (line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    if (colon < 0) continue;
    const field = line.slice(0, colon);
    const value = line.slice(colon + 1).replace(/^ /, "");
    if (field === "event") name = value;
    else if (field === "data") data.push(value);
  }
  if (data.length === 0) return null;

  let parsed: ServerEvent;
  try {
    parsed = JSON.parse(data.join("\n")) as ServerEvent;
  } catch {
    return null;
  }

  if (!name) name = EVENT_NAMES[parsed.type ?? -1] ?? "";
  return name ? { name, data: parsed } : null;
};

/** Split a growing buffer into complete frames, keeping the trailing partial. */
export const drainFrames = (buffer: string): { frames: SseFrame[]; rest: string } => {
  // A proxy may rewrite the daemon's `\n\n` separators as `\r\n\r\n`, which does
  // not contain `\n\n`, so normalise before splitting.
  const parts = buffer.replace(/\r\n/g, "\n").split("\n\n");
  const rest = parts.pop() ?? "";
  const frames: SseFrame[] = [];
  for (const part of parts) {
    const frame = parseEventBlock(part);
    if (frame) frames.push(frame);
  }
  return { frames, rest };
};

export type SseHandlers = {
  onEvent: (frame: SseFrame) => void;
  onStatus: (status: "connecting" | "open" | "closed" | "unauthorized") => void;
};

/**
 * `EventSource` cannot set an Authorization header, so the stream is read off a
 * plain fetch, as the browser extension does. Returns a stop function.
 */
export const streamEvents = (
  serverUrl: string,
  token: string,
  handlers: SseHandlers,
): (() => void) => {
  const abort = new AbortController();
  let retry = 0;
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const scheduleRetry = () => {
    if (stopped) return;
    const delay = Math.min(500 * 2 ** retry++, 10_000);
    timer = setTimeout(() => void run(), delay);
  };

  const run = async () => {
    if (stopped) return;
    handlers.onStatus("connecting");
    let rest = "";
    try {
      // Same trailing-slash strip as the client, or a pasted `.../` breaks only here.
      const response = await fetch(`${serverUrl.replace(/\/+$/, "")}/events`, {
        headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" },
        signal: abort.signal,
      });
      // 401 will not fix itself by retrying, so stop and let the caller re-auth.
      if (response.status === 401) {
        handlers.onStatus("unauthorized");
        return;
      }
      if (!response.ok || !response.body) {
        scheduleRetry();
        return;
      }

      retry = 0;
      handlers.onStatus("open");

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        const drained = drainFrames(rest + decoder.decode(value, { stream: true }));
        rest = drained.rest;
        for (const frame of drained.frames) handlers.onEvent(frame);
      }
    } catch {
      // Aborted, or the daemon went away. Either way, fall through to the retry.
    }
    // A caller-requested stop is not a drop: reporting "closed" here would
    // clobber the status of whichever stream replaced this one.
    if (stopped) return;
    handlers.onStatus("closed");
    scheduleRetry();
  };

  void run();

  return () => {
    stopped = true;
    clearTimeout(timer);
    abort.abort();
  };
};
