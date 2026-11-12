/*
Purpose: Translate explicit session controls and byte events across Wails.
Used by: App, session state and the terminal renderer.
Responsibilities: Describe identities, encode terminal input and track event subscription cleanup.
Important: Register before activation; frame acknowledgements bound native delivery and listener removal never substitutes for backend stop.
*/
import { Events } from "@wailsio/runtime";
import { Service } from "../bindings/github.com/sandjaie/KubeSift/internal/desktop";
export interface ConnectionRef {
  id: string;
  generation: number;
}
export interface SessionRef {
  id: string;
  connection: ConnectionRef;
}
export interface Target {
  connection: ConnectionRef;
  namespace: string;
  name: string;
  container: string;
}
export interface SessionEvent {
  session: SessionRef;
  sequence: number;
  kind: string;
  data: string | null;
}
let listeners = 0;
export function listenerCount(): number {
  return listeners;
}
export function eventBytes(event: SessionEvent): Uint8Array {
  return Uint8Array.from(atob(event.data ?? ""), (c) => c.charCodeAt(0));
}
export function encodeBytes(bytes: Uint8Array): string {
  let value = "";
  for (const byte of bytes) value += String.fromCharCode(byte);
  return btoa(value);
}
export function onSessionEvent(receive: (event: SessionEvent) => void): () => void {
  const off = Events.On("session", (event) => receive(event.data as SessionEvent));
  listeners++;
  let active = true;
  return () => {
    if (active) {
      active = false;
      off();
      listeners--;
    }
  };
}
export async function activateSession(ref: SessionRef): Promise<void> {
  await Service.ActivateSession(ref);
}
export async function stopSession(ref: SessionRef): Promise<void> {
  await Service.StopSession(ref);
}
export async function acknowledge(ref: SessionRef, sequence: number): Promise<void> {
  await Service.AcknowledgeSession(ref, sequence);
}
