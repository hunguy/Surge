import { test } from "node:test";
import assert from "node:assert/strict";

import { hostOf, pillLabel, savedNameFor, serverLabel } from "./server-label.ts";
import type { Connection, SavedConnection } from "../types.ts";

const connection = (over: Partial<Connection> = {}): Connection => ({
  serverUrl: "http://203.0.113.9:1700",
  token: "tok",
  connected: true,
  ...over,
});

const saved = (over: Partial<SavedConnection> = {}): SavedConnection => ({
  name: "Office iMac",
  serverUrl: "http://203.0.113.9:1700",
  token: "tok",
  ...over,
});

test("serverLabel names the saved pair and shows its address", () => {
  assert.equal(serverLabel(connection(), [saved()]), "Office iMac (203.0.113.9:1700)");
});

test("serverLabel falls back to the address alone", () => {
  // Same address, different token: a second connection to the same daemon, which has no name.
  assert.equal(serverLabel(connection(), [saved({ name: "Local", token: "other" })]), "203.0.113.9:1700");
  assert.equal(serverLabel(connection(), []), "203.0.113.9:1700");
  assert.equal(savedNameFor(connection(), []), undefined);
});

test("the pill names the server up and says so in words when down", () => {
  assert.equal(pillLabel(connection(), [saved()]), "Office iMac (203.0.113.9:1700)");
  // The danger tone alone is colour, which the checklist will not let carry the state.
  assert.equal(pillLabel(connection({ connected: false }), [saved()]), "Not connected: 203.0.113.9:1700");
});

test("hostOf keeps a half-typed address readable and drops a default port", () => {
  assert.equal(hostOf("127.0.0.1:1700"), "127.0.0.1:1700");
  assert.equal(hostOf("http://imac.local:80"), "imac.local");
});
