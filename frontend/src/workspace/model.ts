/*
Purpose: Define product presentation states independently of live services.
Used by: Product workspace and shared controls.
Responsibilities: Name distinct loading and failure presentations.
Important: Preview identities never become backend session identities.
*/
export type ViewStatus = 'ready'|'loading'|'empty'|'forbidden'|'disconnected'|'stale'|'unsupported'|'missing-dependency';
