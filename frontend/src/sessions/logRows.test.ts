/*
Purpose: Protect bounded log rows and original search offsets.
Used by: Vitest.
Responsibilities: Cover large history, long lines and Unicode row boundaries.
Important: Tests use synthetic text only.
*/
import { expect, test } from "vitest";
import { logRows } from "./logRows";
test("large log history preserves offsets and bounds each rendered row", () => {
  const text = "first\n" + "😀".repeat(5000) + "\nlast\n";
  const rows = logRows(text);
  expect(rows[0]).toEqual({ start: 0, text: "first" });
  expect(rows[rows.length - 1]?.text).toBe("");
  for (const row of rows) {
    expect(row.text.length).toBeLessThanOrEqual(2048);
    expect(text.slice(row.start, row.start + row.text.length)).toBe(row.text);
    expect(row.text).not.toMatch(/^[\uDC00-\uDFFF]|[\uD800-\uDBFF]$/u);
  }
  expect(rows.find((row) => row.text === "last")?.start).toBe(text.indexOf("last"));
});

test("one maximum-size line is split without losing any retained text", () => {
  const text = "x".repeat(1048576);
  const rows = logRows(text);
  expect(rows).toHaveLength(512);
  expect(rows.map((row) => row.text).join("")).toBe(text);
  expect(rows[511].start).toBe(1046528);
});
