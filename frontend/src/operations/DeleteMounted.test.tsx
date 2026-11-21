// @vitest-environment happy-dom
/*
Purpose: Exercise deletion review and acceptance while the React views are mounted.
Used by: Frontend operation regression tests.
Responsibilities: Control late review, inventory and execution responses across selection changes.
Important: All native calls are mocked and no cluster is used.
*/
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ResourceDetail } from "../workspace/ResourceDetail";
import type { ResourceDetail as CapturedDetail, ResourceRef } from "../workloads/model";
import { emptyKind } from "../workloads/state";
import { useWorkloads, type WorkloadController } from "../workloads/useWorkloads";
import { useOperationResults } from "./OperationResults";
import type { DeletePreview, OperationResult } from "./api";

const bridge = vi.hoisted(() => ({
  prepareDelete: vi.fn(),
  cancel: vi.fn(),
  revokeDelete: vi.fn(),
  executeDelete: vi.fn(),
  executeEdit: vi.fn(),
  executeApply: vi.fn(),
  executeRollback: vi.fn(),
  executeRestart: vi.fn(),
  executeCreate: vi.fn(),
  executeScale: vi.fn(),
  executeNode: vi.fn(),
  listOperationResults: vi.fn(),
  readWorkloads: vi.fn(),
  readResource: vi.fn(),
}));
vi.mock("./api", () => bridge);
vi.mock("@wailsio/runtime", () => ({}));
vi.mock("../workloads/api", () => ({
  readRequest: (read: (id: string) => Promise<unknown>) => ({ promise: read("test-request"), cancel: bridge.cancel }),
  workloadAPI: { ReadWorkloads: bridge.readWorkloads, ReadResource: bridge.readResource },
}));

const resource: ResourceRef = {
  connection: { id: "local", generation: 1 },
  apiVersion: "apps/v1",
  kind: "Deployment",
  namespace: "fixture",
  name: "web",
  uid: "original",
};
const row = { resource };
const detail = { resource, objectJSON: '{"metadata":{},"spec":{}}', resourceVersion: "7" } as CapturedDetail;

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.clearAllMocks();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((complete, fail) => {
    resolve = complete;
    reject = fail;
  });
  return { promise, resolve, reject };
}

async function click(label: string) {
  const button = Array.from(host.querySelectorAll("button")).find((candidate) => candidate.textContent === label);
  expect(button, label).toBeDefined();
  await act(async () => button?.click());
}

test("a mounted review cancels and revokes late tokens on captured source changes", async () => {
  const render = (capture: CapturedDetail, tab = "A", refresh = 0) => {
    const controller = {
      selection: { resource: capture.resource },
      activeSection: "Overview",
      sections: { Overview: { data: capture, loading: false, error: "" } },
      inventory: { ...emptyKind(), sectionRefresh: refresh },
    } as WorkloadController;
    root.render(
      <ResourceDetail key={tab} controller={controller} context="local" stale={false} canReviewOperation onOperation={vi.fn()} />,
    );
  };
  const changes = [
    { capture: detail, refresh: 1 },
    { capture: { ...detail, resourceVersion: "8" } },
    { capture: { ...detail, resource: { ...resource, uid: "replacement" } } },
    { capture: { ...detail, resource: { ...resource, namespace: "other" } } },
    { capture: { ...detail, resource: { ...resource, connection: { ...resource.connection, generation: 2 } } } },
    { capture: detail, tab: "B" },
  ];
  for (const [index, { capture, tab, refresh }] of changes.entries()) {
    await act(async () => render(detail));
    const pending = deferred<DeletePreview>();
    const cancel = vi.fn();
    bridge.prepareDelete.mockReturnValueOnce({ promise: pending.promise, cancel });
    const menu = host.querySelector<HTMLSelectElement>('select[aria-label="Deployment actions"]');
    expect(menu).not.toBeNull();
    expect([...menu!.options].map((option) => option.text)).toEqual(["Actions", "Scale…", "Restart…", "Delete…"]);
    await act(async () => {
      menu!.value = "delete";
      menu!.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await click("Review deletion");
    expect(host.querySelector("dialog")).not.toBeNull();
    await act(async () => render(capture, tab, refresh));
    expect(cancel).toHaveBeenCalledOnce();
    expect(host.querySelector("dialog")).toBeNull();
    await act(async () => pending.resolve({ token: `late-${index}` } as DeletePreview));
    expect(bridge.revokeDelete).toHaveBeenCalledWith(`late-${index}`);
  }
});

const accepted: OperationResult = {
  action: "delete",
  resource,
  cluster: "local",
  operationID: "operation",
  state: "accepted",
  startedAt: "",
  finishedAt: "2026-11-16T04:00:00Z",
  delete: { propagation: "Background", gracePolicy: "Server default" },
  restart: null,
  scale: null,
  problem: null,
};

test.each(["inventory first", "acceptance first", "explicit selection", "lost reply"] as const)(
  "the mounted inspector handles %s",
  async (order) => {
    const page = deferred<{ items: { resource: ResourceRef }[] }>();
    const execution = deferred<OperationResult>();
    bridge.readWorkloads.mockResolvedValueOnce({ items: [row] }).mockReturnValue(page.promise);
    bridge.readResource.mockResolvedValue(detail);
    bridge.executeDelete.mockReturnValueOnce(execution.promise);
    bridge.listOperationResults.mockResolvedValue([]);

    let controller!: WorkloadController;
    let operations!: ReturnType<typeof useOperationResults>;
    function WorkspaceFlow() {
      controller = useWorkloads("A", resource.connection, resource.namespace, true, () => {});
      operations = useOperationResults((result) => {
        if (result.state === "accepted") controller.refreshOperationTarget(result.resource, result.action === "delete");
      });
      return null;
    }
    await act(async () => root.render(<WorkspaceFlow />));
    await act(async () => controller.selectKind("Deployment"));
    await act(async () => controller.selectResource(row));
    await act(async () => {
      operations.submit("delete", "review", resource.name, controller.holdDeletionSelection());
      controller.refresh();
    });
    const replacement = { resource: { ...resource, uid: "replacement" } };
    if (order === "acceptance first") await act(async () => execution.resolve(accepted));
    if (order === "lost reply") await act(async () => execution.reject(new Error("bridge response lost")));
    await act(async () => page.resolve({ items: [replacement] }));
    if (order === "inventory first") {
      expect(controller.selection?.resource).toEqual(resource);
      await act(async () => execution.resolve(accepted));
    }
    if (order === "lost reply") {
      expect(controller.selection).toBeNull();
      bridge.listOperationResults.mockResolvedValue([accepted]);
      await act(async () => operations.refresh());
    }
    if (order === "explicit selection") {
      await act(async () => controller.selectResource(replacement));
      await act(async () => execution.resolve(accepted));
      expect(controller.inventory.retainedAfterDeletion).toBe(false);
      expect(controller.selection?.resource).toEqual(replacement.resource);
    } else {
      expect(controller.inventory.retainedAfterDeletion).toBe(true);
      expect(controller.selection?.resource).toEqual(resource);
    }
    await act(async () => controller.backToInventory());
    expect(controller.selection).toBeNull();
  },
);

test.each(["completed", "partial", "unknown"])("a terminal Node %s refreshes its target once after background polling", async (state) => {
  const finished = { ...accepted, action: "node", state, resource: { ...resource, kind: "Node", apiVersion: "v1", namespace: "" } };
  bridge.listOperationResults.mockResolvedValue([{ ...finished, state: "executing", finishedAt: null }]);
  const onFinish = vi.fn();
  let operations!: ReturnType<typeof useOperationResults>;
  function ResultsController() {
    operations = useOperationResults(onFinish);
    return null;
  }
  await act(async () => root.render(<ResultsController />));
  expect(onFinish).not.toHaveBeenCalled();
  bridge.listOperationResults.mockResolvedValue([finished]);
  await act(async () => operations.refresh());
  await act(async () => operations.refresh());
  expect(onFinish).toHaveBeenCalledTimes(1);
  expect(onFinish).toHaveBeenCalledWith(finished);
});
