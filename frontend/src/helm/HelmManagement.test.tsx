// @vitest-environment happy-dom
/*
Purpose: Exercise the mounted Helm review and result journey.
Used by: Frontend regression tests for native Helm management.
Responsibilities: Check edited values, failed and late previews, one confirmation and retained operation status.
Important: The bridge is mocked; live behavior is covered by native and packaged verification.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { HelmActionDialog } from "./HelmActionDialog";
import { HelmOperationResults, useHelmOperationResults } from "./HelmOperationResults";
import type { Review } from "../../bindings/internal/helmmanager/models";

const bridge = vi.hoisted(() => ({
  helmManagementAPI: {
    ListHelmSources: vi.fn(),
    PickHelmChart: vi.fn(),
    DiscardHelmDraft: vi.fn(),
    ReviseHelmReview: vi.fn(),
    ListHelmResults: vi.fn(),
    ExecuteHelmAction: vi.fn(),
    CancelHelmAction: vi.fn(),
  },
  resolveHelmChart: vi.fn(),
  prepareHelmReview: vi.fn(),
  loadUpgradeValues: vi.fn(),
  listHelmCharts: vi.fn(),
  importHelmValues: vi.fn(),
}));
vi.mock("./managementApi", () => bridge);
vi.mock("@wailsio/runtime", () => ({}));
vi.mock("./api", () => ({ readHistory: vi.fn() }));
const target = { connection: { id: "cluster-a", generation: 1 }, namespace: "owned", driver: "secret", releaseName: "demo" };
const current = { ...target, revision: "2", recordName: "sh.helm.release.v1.demo.v2", uid: "uid", resourceVersion: "1" };
const review: Review = {
  token: "review",
  target,
  contextName: "kind-dev-local-a",
  action: "upgrade",
  chartName: "demo",
  chartVersion: "2.0.0",
  digest: "sha256:chart",
  expiresAt: new Date(Date.now() + 300000).toISOString(),
  current,
  rollback: null,
  options: { timeoutSeconds: 300, disableHooks: false, createNamespace: false, keepHistory: false },
  warnings: ["Helm preview only; execution may differ."],
  sections: ["values", "manifest"],
};
const response = {
  ...review,
  content: { currentValues: "message: old", values: "message: edited", currentManifest: "old", manifest: "new" },
};
const request = <T,>(value: T) => ({ promise: Promise.resolve(value), cancel: vi.fn() });
let host: HTMLDivElement;
let root: ReturnType<typeof createRoot>;
const confirm = vi.fn();
function button(label: string) {
  return [...host.querySelectorAll("button")].find((node) => node.textContent?.trim() === label)!;
}
async function render(valid = true) {
  await act(async () =>
    root.render(
      <HelmActionDialog
        action="upgrade"
        target={target}
        contextLabel="kind-dev-local-a"
        revision={current}
        valid={valid}
        onClose={vi.fn()}
        onConfirm={confirm}
      />,
    ),
  );
}
async function prepare() {
  await act(async () => button("Select packaged chart").click());
  const values = host.querySelector<HTMLTextAreaElement>('textarea[aria-label="Override values"]')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(values, "message: edited\n");
    values.dispatchEvent(new Event("input", { bubbles: true }));
    values.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await act(async () => button("Prepare review").click());
}
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.clearAllMocks();
  host = document.body.appendChild(document.createElement("div"));
  root = createRoot(host);
  bridge.helmManagementAPI.ListHelmSources.mockResolvedValue([]);
  bridge.helmManagementAPI.PickHelmChart.mockResolvedValue("chart");
  bridge.helmManagementAPI.DiscardHelmDraft.mockResolvedValue(undefined);
  bridge.loadUpgradeValues.mockReturnValue(request("message: old\n"));
  bridge.resolveHelmChart.mockReturnValue(
    request({ id: "chart", name: "demo", version: "2.0.0", digest: "sha256:chart", expiresAt: review.expiresAt }),
  );
  bridge.prepareHelmReview.mockImplementation(() => request(response));
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

test("edited upgrade values produce one fixed-target confirmation", async () => {
  await render();
  expect(document.activeElement?.textContent).toBe("Cancel");
  await prepare();
  expect(bridge.prepareHelmReview.mock.calls[0][0].valuesYAML).toBe("message: edited\n");
  expect(host.textContent).toContain("kind-dev-local-a / owned / demo");
  expect(host.textContent).toContain("Helm preview only");
  await act(async () => {
    button("Confirm upgrade").click();
    button("Confirm upgrade").click();
  });
  expect(confirm).toHaveBeenCalledExactlyOnceWith("review");
});
test("failed preparation announces an error and cannot confirm", async () => {
  bridge.prepareHelmReview.mockImplementation(() => ({ promise: Promise.reject(new Error("Synthetic forbidden")), cancel: vi.fn() }));
  await render();
  await prepare();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain("Synthetic forbidden");
  expect(button("Confirm upgrade")).toBeUndefined();
});
test("navigation revokes a late preview and does not revive confirmation", async () => {
  let resolve!: (value: typeof response) => void;
  bridge.prepareHelmReview.mockReturnValue({
    promise: new Promise((done) => {
      resolve = done;
    }),
    cancel: vi.fn(),
  });
  await render();
  await prepare();
  await render(false);
  await act(async () => resolve(response));
  await render(true);
  expect(button("Confirm upgrade")).toBeUndefined();
  expect(bridge.helmManagementAPI.DiscardHelmDraft).toHaveBeenCalledWith("review");
  expect(confirm).not.toHaveBeenCalled();
});
test("results retain the captured target after dialog navigation", async () => {
  const result = {
    id: "running",
    action: "upgrade",
    target,
    contextName: "kind-dev-local-a",
    state: "running",
    stage: "Waiting for readiness",
    startedAt: "",
    finishedAt: null,
    observed: null,
    problem: null,
  };
  bridge.helmManagementAPI.ListHelmResults.mockResolvedValue([result]);
  function Harness({ page }: { page: string }) {
    const status = useHelmOperationResults();
    return (
      <>
        <p>{page}</p>
        <HelmOperationResults
          results={status.results}
          pending={status.pending}
          error={status.error}
          onRefresh={status.refresh}
          onCancel={vi.fn()}
          onTarget={vi.fn()}
          canOpen={() => false}
        />
      </>
    );
  }
  vi.useFakeTimers();
  await act(async () => root.render(<Harness page="Helm workspace" />));
  await act(async () => root.render(<Harness page="Other cluster workspace" />));
  expect(host.textContent).toContain("kind-dev-local-a / owned / demo");
  expect(host.textContent).toContain("Waiting for readiness");
  expect(button("Cancel Helm operation")).toBeDefined();
  bridge.helmManagementAPI.ListHelmResults.mockResolvedValue([{ ...result, state: "succeeded", stage: "Helm action completed" }]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(host.textContent).toContain("Helm action completed");
  expect(bridge.helmManagementAPI.ExecuteHelmAction).not.toHaveBeenCalled();
});
