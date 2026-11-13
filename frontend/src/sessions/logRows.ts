/*
Purpose: Index retained log text into bounded display rows.
Used by: LogViewer and its regression test.
Responsibilities: Preserve original offsets for search and split pathological long lines.
Important: Each row is at most 2048 UTF-16 units and never splits a surrogate pair.
*/
export function logRows(text: string) {
  const rows: { start: number; text: string }[] = [];
  let start = 0;
  let newline = text.indexOf("\n");
  while (start < text.length) {
    const lineEnd = newline < 0 ? text.length : newline;
    let end = Math.min(lineEnd, start + 2048);
    if (end < lineEnd && /[\uD800-\uDBFF]/u.test(text[end - 1])) end--;
    rows.push({ start, text: text.slice(start, end) });
    start = end === newline ? end + 1 : end;
    if (end === newline) newline = text.indexOf("\n", start);
  }
  if (!text || text.endsWith("\n")) rows.push({ start: text.length, text: "" });
  return rows;
}
