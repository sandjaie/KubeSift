/*
Purpose: Identify Helm reads and verify the revision echoed by the reader.
Used by: useHelm and its request ownership test.
Responsibilities: Keep release and scope identities exact across changes.
Important: Precision and connection generations are part of selection identity.
*/
import type { RecordRef } from "../../bindings/internal/helmreader/models";

export const sections = ["Overview", "History", "Values", "Manifest", "Hooks", "Notes", "Resources"] as const;
export type Section = (typeof sections)[number];

function recordKey(record: RecordRef) {
  return [
    record.connection.id,
    record.connection.generation,
    record.namespace,
    record.driver,
    record.releaseName,
    record.revision,
    record.recordName,
    record.uid,
    record.resourceVersion,
  ];
}

export const matchesRecord = (a: RecordRef, b: RecordRef) => JSON.stringify(recordKey(a)) === JSON.stringify(recordKey(b));

export function readKey(tab: string, record: RecordRef, section: Section, computed: boolean, page: number, refresh: number) {
  return JSON.stringify([tab, recordKey(record), section, computed, page, refresh]);
}
