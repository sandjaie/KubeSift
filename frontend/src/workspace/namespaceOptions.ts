/*
Purpose: Build safe choices for the searchable namespace control.
Used by: NamespaceControl and its focused regression tests.
Responsibilities: Filter known names and offer explicit entry when discovery is incomplete.
Important: Selection remains an explicit action; this module never makes API requests.
*/
export function namespaceOptions(query: string, names: string[], complete: boolean, current: string) {
  const term = query.trim();
  const known = [...new Set([current, ...names])].filter(Boolean).sort();
  const options = known.filter((name) => name.toLowerCase().includes(term.toLowerCase())).map((value) => ({ value, manual: false }));
  if (!complete && term.length <= 63 && /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(term) && !known.includes(term)) {
    options.push({ value: term, manual: true });
  }
  return options;
}
