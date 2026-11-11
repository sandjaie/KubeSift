/*
Purpose: Time fixed terminal probes at the xterm parse callback.
Used by: Terminal and the explicit packaged renderer validation profile.
Responsibilities: Exclude echoed command text from RTT timing and bound pending samples/output.
Important: Send only a fixed synthetic printf probe to an explicit session; never replay or expose a local command runner.
*/
import { Service } from "../bindings/github.com/sandjaie/KubeSift/internal/desktop";
import { encodeBytes, type SessionRef } from "./bridge";
const ready = new Set<string>();
let pending: { id: string; marker: string; tail: string; start: number; done: (value: number) => void } | undefined;
export function terminalReady(ref: SessionRef, value: boolean) {
  if (value) ready.add(ref.id);
  else ready.delete(ref.id);
}
export function terminalParsed(ref: SessionRef, bytes: Uint8Array) {
  if (!pending || pending.id !== ref.id) return;
  pending.tail = (pending.tail + new TextDecoder().decode(bytes)).slice(-2048);
  if (pending.tail.includes(pending.marker)) {
    const sample = pending;
    pending = undefined;
    sample.done(performance.now() - sample.start);
  }
}
export async function terminalSamples(ref: SessionRef): Promise<number[]> {
  const until = performance.now() + 10000;
  while (!ready.has(ref.id)) {
    if (performance.now() > until) throw new Error("Validation terminal did not activate");
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  const samples: number[] = [];
  for (let i = 0; i < 20; i++) {
    const token = crypto.randomUUID().replace(/-/g, "");
    samples.push(
      await new Promise<number>((resolve, reject) => {
        const timeout = setTimeout(() => {
          pending = undefined;
          reject(new Error("Validation terminal probe timed out"));
        }, 10000);
        pending = {
          id: ref.id,
          marker: `__KS_RTT_${token}__`,
          tail: "",
          start: performance.now(),
          done: (value) => {
            clearTimeout(timeout);
            resolve(value);
          },
        };
        void Service.TerminalInput(ref, encodeBytes(new TextEncoder().encode(`printf '__KS_RTT_%s__\\n' '${token}'\n`))).catch(() => {
          clearTimeout(timeout);
          pending = undefined;
          reject(new Error("Validation probe input failed"));
        });
      }),
    );
  }
  return samples;
}
