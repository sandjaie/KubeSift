/*
Purpose: Describe transient presentation state for live cluster tabs.
Used by: The connection reducer and product workspace.
Responsibilities: Keep native identity separate from tab selection and namespace read epochs.
Important: No connection state is persisted or automatically restored on launch.
*/
import type { CatalogEntry, ConnectionView, SourceResult } from "./api";
export interface ConnectionTab {
  entry: CatalogEntry;
  request: number;
  attemptId: string;
  epoch: number;
  namespace: string;
  filter: string;
  selected: string | null;
  view: ConnectionView | null;
  error: string;
  pollFailed: boolean;
}
export interface ConnectionsState {
  sources: SourceResult[];
  tabs: Record<string, ConnectionTab>;
  active: string | null;
  error: string;
  loading: boolean;
}
