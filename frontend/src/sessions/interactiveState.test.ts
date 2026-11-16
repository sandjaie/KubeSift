/*
Purpose: Test interactive ownership across navigation and asynchronous setup.
Used by: Focused frontend session tests.
Responsibilities: Verify registration, activation, late cancellation and retained entry limits.
Important: The synthetic bridge never starts a native session.
*/
import { expect, it, vi } from "vitest";
import type { SessionEvent } from "../bridge";
import type { InteractiveEntry } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";
import { createInteractiveStore } from "./interactiveState";

it("keeps one fixed owner across view changes and releases late or retained entries explicitly", async () => {
  let receive!: (event: SessionEvent) => void;
  const subscribe = vi.fn((handler: typeof receive) => {
    receive = handler;
    return vi.fn();
  });
  const bridge = { subscribe, activate: vi.fn(async () => {}), stop: vi.fn(async () => {}), close: vi.fn(async () => {}) };
  const owner = createInteractiveStore(bridge);
  owner.attach();
  owner.attach();
  expect(subscribe).toHaveBeenCalledOnce();
  const connection = { id: "a", generation: 4 };
  const resource = { connection, apiVersion: "v1", kind: "Pod", namespace: "demo", name: "web", uid: "original" };
  const entry = (id: string): InteractiveEntry => ({
    session: { id, connection },
    kind: "forward",
    target: { resource, container: "" },
    service: null,
    localPort: 0,
    remotePort: 8080,
    state: "connecting",
    problem: null,
  });
  const first = entry("first");
  const request = (value: InteractiveEntry) => () => ({ promise: Promise.resolve(value), cancel: vi.fn() });
  await owner.open("tab-a", "A", resource, request(first));
  owner.activate(first.session);
  owner.activate(first.session);
  expect(bridge.activate).toHaveBeenCalledExactlyOnceWith(first.session);
  const state = (state: string, generation = 4, sequence = 1) =>
    receive({
      session: { id: "first", connection: { ...connection, generation } },
      sequence,
      kind: "state",
      data: btoa(JSON.stringify({ state, localPort: 1234 })),
    });
  state("listening", 5);
  expect(owner.snapshot()[0].state).toBe("connecting");
  state("listening");
  expect(owner.snapshot()[0]).toMatchObject({ target: first.target, tab: "tab-a", localPort: 1234, state: "listening" });
  state("ended", 4, 2);
  expect(owner.snapshot()[0]).toMatchObject({ state: "ended", localPort: 0 });
  const cancel = vi.fn();
  let late!: (value: InteractiveEntry) => void;
  const open = owner.open("tab-b", "B", resource, () => ({ promise: new Promise((resolve) => (late = resolve)), cancel }));
  const failure = expect(open).rejects.toThrow();
  await owner.closeTab("tab-b");
  late(entry("late"));
  await failure;
  expect(cancel).toHaveBeenCalledOnce();
  expect(bridge.close).toHaveBeenCalledWith(entry("late").session);
  for (let i = 1; i < 8; i++) await owner.open("tab-a", "A", resource, request(entry(String(i))));
  await expect(owner.open("tab-a", "A", resource, request(entry("ninth")))).rejects.toThrow(/maximum eight/);
  await owner.stop("1");
  expect(owner.snapshot().find((s) => s.session.id === "1")?.state).toBe("stopped");
  await owner.close("1");
  await owner.open("tab-a", "A", resource, request(entry("reused-slot")));
  expect(owner.snapshot()).toHaveLength(8);
  await owner.dispose();
});
