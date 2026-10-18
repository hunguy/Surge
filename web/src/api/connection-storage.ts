import type { Connection, SavedConnection } from "../types.ts";

/** The port `surge server` binds when none is given. See bindServerListener. */
export const DEFAULT_CONNECTION: Connection = {
  serverUrl: "http://127.0.0.1:1700",
  token: "",
  connected: false,
};

const STORAGE_KEY = "surge-connection";
/** The key the mock used before it talked to a real daemon. */
const LEGACY_STORAGE_KEY = "surge-mock-connection";

/**
 * Read the stored connection. Called from an effect after mount, never during
 * the first render, so the server pass and the client pass agree.
 */
export const readStoredConnection = (): Connection => {
  try {
    const raw =
      window.localStorage.getItem(STORAGE_KEY) ?? window.localStorage.getItem(LEGACY_STORAGE_KEY);
    if (!raw) return DEFAULT_CONNECTION;
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return DEFAULT_CONNECTION;
    const { serverUrl, token } = parsed as Partial<Connection>;
    if (typeof serverUrl !== "string" || typeof token !== "string") return DEFAULT_CONNECTION;
    if (!serverUrl || !token) return DEFAULT_CONNECTION;
    return { serverUrl, token, connected: true };
  } catch {
    return DEFAULT_CONNECTION;
  }
};

export const writeStoredConnection = (connection: Connection): void => {
  try {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ serverUrl: connection.serverUrl, token: connection.token }),
    );
  } catch {
    // A blocked storage API only means the app forgets between reloads.
  }
};

const SAVED_KEY = "surge-saved-connections";

/** Read the named connections. Called after mount, like readStoredConnection. */
export const readSavedConnections = (): SavedConnection[] => {
  try {
    const raw = window.localStorage.getItem(SAVED_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.flatMap(entry => {
      if (typeof entry !== "object" || entry === null) return [];
      const { name, serverUrl, token } = entry as Partial<SavedConnection>;
      if (!name || typeof serverUrl !== "string" || typeof token !== "string") return [];
      return [{ name, serverUrl, token }];
    });
  } catch {
    return [];
  }
};

/** Saved under the name alone: reusing a name overwrites that tile rather than stacking a duplicate. */
export const saveStoredConnection = (saved: SavedConnection): SavedConnection[] => {
  const next = [...readSavedConnections().filter(entry => entry.name !== saved.name), saved];
  writeSavedConnections(next);
  return next;
};

export const removeStoredConnection = (name: string): SavedConnection[] => {
  const next = readSavedConnections().filter(entry => entry.name !== name);
  writeSavedConnections(next);
  return next;
};

const writeSavedConnections = (saved: SavedConnection[]): void => {
  try {
    window.localStorage.setItem(SAVED_KEY, JSON.stringify(saved));
  } catch {
    // A blocked storage API only means the app forgets between reloads.
  }
};
