import type { Connection, SavedConnection } from "../types.ts";

/**
 * The address alone. The scheme and path never tell two servers apart, and a
 * non-default port survives, because 1700 and a proxy in front of it are two
 * different servers to this client.
 */
export const hostOf = (serverUrl: string): string => {
  try {
    return new URL(serverUrl).host;
  } catch {
    // A half-typed address is still worth showing. It reads better than "invalid URL".
    return serverUrl;
  }
};

/** The name this browser saved for that exact pair, or nothing if it was never saved. */
export const savedNameFor = (connection: Connection, saved: SavedConnection[]): string | undefined =>
  saved.find(entry => entry.serverUrl === connection.serverUrl && entry.token === connection.token)?.name;

/**
 * The pill while the daemon is answering. The tone already says the connection is
 * healthy, so the text is free to say which server it is: the saved name with the
 * address beside it, or the address alone when the pair was never saved.
 */
export const serverLabel = (connection: Connection, saved: SavedConnection[]): string => {
  const host = hostOf(connection.serverUrl);
  const name = savedNameFor(connection, saved);
  return name ? `${name} (${host})` : host;
};

/** The pill at rest. A down connection says so in words, since the tone alone is colour. */
export const pillLabel = (connection: Connection, saved: SavedConnection[]): string =>
  connection.connected ? serverLabel(connection, saved) : `Not connected: ${hostOf(connection.serverUrl)}`;
