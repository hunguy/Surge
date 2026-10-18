import { test } from "node:test";
import assert from "node:assert/strict";

import { partsFor } from "./number-parts.ts";

const text = (value: number, decimals = 0) => partsFor(value, decimals, "en-US").map(part => "digit" in part ? String(part.digit) : part.text).join("");
const separators = (value: number, decimals = 0) => partsFor(value, decimals, "en-US").flatMap(part => "digit" in part || !part.separator ? [] : [part.text]);

// The rendered value must always equal the formatted number, comma included.
test("parts re-join into the formatted number", () => {
  assert.equal(text(7), "7");
  assert.equal(text(999), "999");
  assert.equal(text(1000), "1,000");
  assert.equal(text(1234567), "1,234,567");
  assert.equal(text(1234.5, 1), "1,234.5");
});

// A comma is part of the figure, a suffix is a unit: they render at different sizes, so they must not share a class.
test("group and decimal marks are separators, not symbols", () => {
  assert.deepEqual(separators(1000), [","]);
  assert.deepEqual(separators(1234.5, 1), [",", "."]);
  assert.deepEqual(separators(999), []);
  assert.equal(partsFor(1000, 0, "en-US").flatMap(part => "digit" in part || !part.separator ? [] : [part.text]).join(""), ",");
});

// 999 → 1,000: the new comma takes the place of the thousands digit without disturbing the ones column.
test("columns stay keyed by place value as the number grows", () => {
  const keys = (value: number) => partsFor(value, 0, "en-US").map(part => part.key);
  assert.deepEqual(keys(999), ["i2", "i1", "i0"]);
  assert.deepEqual(keys(1000), ["i3", "g3", "i2", "i1", "i0"]);
});
