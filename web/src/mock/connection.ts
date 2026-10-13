import type { MockConnection } from "./types";

export const DEFAULT_CONNECTION: MockConnection = {
  serverUrl: "http://127.0.0.1:8181",
  token: "mock-token",
  connected: true,
};

const STORAGE_KEY = "surge-mock-connection";

/**
 * Read the stored connection. Called from an effect after mount, never during
 * the first render, so the server pass and the client pass agree.
 */
export const readStoredConnection = (): MockConnection => {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_CONNECTION;
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return DEFAULT_CONNECTION;
    const { serverUrl, token } = parsed as Partial<MockConnection>;
    if (typeof serverUrl !== "string" || typeof token !== "string") return DEFAULT_CONNECTION;
    return { serverUrl, token, connected: Boolean(serverUrl && token) };
  } catch {
    return DEFAULT_CONNECTION;
  }
};

export const writeStoredConnection = (connection: MockConnection): void => {
  try {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ serverUrl: connection.serverUrl, token: connection.token }),
    );
  } catch {
    // A blocked storage API only means the mock forgets between reloads.
  }
};
