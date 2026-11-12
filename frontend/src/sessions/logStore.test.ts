/*
Purpose: Protect bounded shared log retention and native lifecycle ownership.
Used by: Vitest.
Responsibilities: Cover split UTF-8, stale segments, clear/loss and stop/close capacity.
Important: The fake bridge exercises the real store without a native window.
*/
import { expect, test } from "vitest";
import { createLogStore, displayWindow, type LogBridge } from "./logStore";
import type { SessionEvent } from "../bridge";
import type { LogRequest } from "../workloads/model";
const req: LogRequest = {
  resource: { connection: { id: "a", generation: 1 }, apiVersion: "v1", kind: "Pod", namespace: "default", name: "pod", uid: "uid" },
  container: "main",
  previous: false,
  follow: true,
  tailLines: 200,
};
test("one shared subscription bounds UTF-8 retention, rejects late frames and separates stop from close", async () => {
  let receive: (e: SessionEvent) => void = () => {};
  let subscriptions = 0,
    opens = 0,
    acks = 0,
    closes = 0;
  const bridge: LogBridge = {
    subscribe: (fn) => {
      receive = fn;
      subscriptions++;
      return () => {
        subscriptions--;
      };
    },
    open: async () => ({ id: `native-${++opens}`, connection: req.resource.connection }),
    activate: async () => {},
    ack: async () => {
      acks++;
    },
    stop: async () => {},
    close: async () => {
      closes++;
    },
  };
  const store = createLogStore(bridge);
  store.attach();
  const offA = store.subscribe(() => {}),
    offB = store.subscribe(() => {});
  const id = await store.open(req, "A", "local-a");
  const ref = store.snapshot()[0].ref!;
  let sequence = 0;
  const send = (bytes: Uint8Array, kind = "data") =>
    receive({ session: ref, sequence: ++sequence, kind, data: btoa(Array.from(bytes, (b) => String.fromCharCode(b)).join("")) });
  const bytes = new TextEncoder().encode("€");
  send(bytes.slice(0, 1));
  send(bytes.slice(1));
  expect(store.snapshot()[0].text).toBe("€");
  const block = new TextEncoder().encode("x".repeat(65536));
  for (let n = 0; n < 18; n++) send(block);
  const retained = store.snapshot()[0];
  expect(new TextEncoder().encode(retained.text).length).toBeLessThanOrEqual(1048576);
  expect(retained.dropped).toBeGreaterThan(0);
  expect(new TextEncoder().encode(displayWindow(retained.text, 0)).length).toBeLessThanOrEqual(65536);
  offA();
  offB();
  send(new TextEncoder().encode("offscreen"));
  expect(acks).toBe(21);
  expect(subscriptions).toBe(1);
  expect(opens).toBe(1);
  store.clear(id);
  expect(store.snapshot()[0].text).toBe("");
  expect(store.snapshot()[0].dropped).toBeGreaterThan(0);
  await store.stop(id);
  send(new TextEncoder().encode("late"));
  expect(store.snapshot()[0].text).toBe("");
  expect(store.snapshot()[0].status).toBe("stopped");
  for (let n = 0; n < 7; n++) await store.open(req, "A", "local-a");
  await expect(store.open(req, "B", "local-b")).rejects.toThrow(/eight/);
  await store.close(id);
  expect(store.snapshot()).toHaveLength(7);
  expect(closes).toBe(1);
  await store.dispose();
  expect(subscriptions).toBe(0);
});

test("concurrent Resume and late opens cannot orphan a native segment", async () => {
  let serial = 0;
  const releases: (() => void)[] = [];
  const live = new Set<string>();
  const bridge: LogBridge = {
    subscribe: () => () => {},
    open: async () => {
      const id = `segment-${++serial}`;
      live.add(id);
      return { id, connection: req.resource.connection };
    },
    activate: async () => {},
    ack: async () => {},
    stop: async () => {},
    close: async (ref) => {
      if (ref.id === "segment-1")
        await new Promise<void>((resolve) => {
          releases.push(resolve);
        });
      live.delete(ref.id);
    },
  };
  const store = createLogStore(bridge);
  store.attach();
  const id = await store.open(req, "A", "local-a");
  await store.stop(id);
  const first = store.resume(id),
    second = store.resume(id);
  await Promise.resolve();
  releases.forEach((fn) => fn());
  await Promise.all([first, second]);
  expect(serial).toBe(2);
  await store.close(id);
  expect(live.size).toBe(0);
  await store.dispose();
});

test("auto-scroll follows the viewport only while enabled", async () => {
  const { scrollLogViewport } = await import("./logStore");
  const viewport = { scrollTop: 10, scrollHeight: 500 };
  scrollLogViewport(viewport, true);
  expect(viewport.scrollTop).toBe(500);
  viewport.scrollTop = 20;
  scrollLogViewport(viewport, false);
  expect(viewport.scrollTop).toBe(20);
});

test("a stopped pending open cannot attach to a resumed segment", async () => {
  const pending: ((ref: { id: string; connection: typeof req.resource.connection }) => void)[] = [];
  const closed: string[] = [];
  const bridge: LogBridge = {
    subscribe: () => () => {},
    open: () => new Promise((resolve) => pending.push(resolve)),
    activate: async () => {},
    ack: async () => {},
    stop: async () => {},
    close: async (ref) => {
      closed.push(ref.id);
    },
  };
  const store = createLogStore(bridge);
  store.attach();
  const opening = store.open(req, "A", "local-a");
  const id = store.snapshot()[0].id;
  await store.stop(id);
  const resuming = store.resume(id);
  pending[1]({ id: "new", connection: req.resource.connection });
  await resuming;
  pending[0]({ id: "old", connection: req.resource.connection });
  await opening;
  expect(store.snapshot()[0].ref?.id).toBe("new");
  expect(closed).toContain("old");
  await store.dispose();
  expect(closed).toContain("new");
});
