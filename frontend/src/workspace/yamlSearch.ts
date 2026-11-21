/*
Purpose: Search a read-only YAML capture literally.
Used by: The workload YAML viewer.
Responsibilities: Preserve source offsets and bound the number of indexed matches.
Important: Matches never change the displayed or copied document.
*/
export const yamlMatchLimit = 10000;
export function yamlMatches(text: string, query: string) {
  if (!query) return [];
  const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const offsets: number[] = [];
  for (const match of text.matchAll(new RegExp(escaped, "giu"))) {
    offsets.push(match.index);
    if (offsets.length > yamlMatchLimit) break;
  }
  return offsets;
}
