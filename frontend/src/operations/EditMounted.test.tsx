// @vitest-environment happy-dom
/*
Purpose: Exercise mounted YAML editor replacement and refresh behavior.
Used by: Frontend operation regressions.
Responsibilities: Simulate deferred bridge responses and protected draft selection.
Important: These tests use only synthetic data and mocked native calls.
*/
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { APIResourceBrowser } from "../discovery/APIResourceBrowser";
import type { CatalogSeed } from "../discovery/model";
import { EditDialog } from "./EditDialog";
import { requestEditor, type EditorSession } from "./editState";
import type { OperationResult } from "./api";

const bridge = vi.hoisted(() => ({
  closeEdit: vi.fn(),
  openEdit: vi.fn(),
  prepareEdit: vi.fn(),
  revokeEdit: vi.fn(),
  groups: vi.fn(),
  resources: vi.fn(),
  inventory: vi.fn(),
  detail: vi.fn(),
}));
vi.mock("./api", () => bridge);
vi.mock("@wailsio/runtime", () => ({}));
vi.mock("../workloads/api", () => ({
  readRequest: (read: (id: string) => Promise<unknown>) => ({ promise: read("request"), cancel: vi.fn() }),
  workloadAPI: {
    ReadAPIGroups: bridge.groups,
    ReadAPIResources: bridge.resources,
    ReadGenericResources: bridge.inventory,
    ReadGenericDetail: bridge.detail,
  },
}));

const target = {
  connection: { id: "a", generation: 1 },
  group: "",
  version: "v1",
  resource: "secrets",
  kind: "Secret",
  namespace: "demo",
  name: "sample",
  uid: "a-id",
};
const other = { ...target, connection: { id: "b", generation: 1 }, uid: "b-id" };
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

async function click(label: string) {
  const button = Array.from(host.querySelectorAll("button")).find((candidate) => candidate.textContent === label);
  expect(button, label).toBeDefined();
  await act(async () => button?.click());
}

async function type(text: string) {
  const editor = host.querySelector("textarea")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(editor, text);
    editor.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

const props = {
  target,
  context: "a",
  connection: target.connection,
  suspended: false,
  reveal: false,
  closing: false,
  onSuspend: vi.fn(),
  onCancelClose: vi.fn(),
  onClose: vi.fn(),
  onConfirm: vi.fn(),
};

test("a failed fresh read cannot bring back a discarded Secret draft", async () => {
  bridge.openEdit.mockReturnValueOnce({ promise: Promise.resolve({ target, id: "old", yaml: "data: original" }), cancel: vi.fn() });
  bridge.openEdit.mockImplementationOnce(() => ({
    promise: Promise.reject(new Error("Kubernetes denied this request.")),
    cancel: vi.fn(),
  }));
  await act(async () => root.render(<EditDialog {...props} />));
  await type("data: unsaved");
  await click("Refresh from Kubernetes");
  await click("Discard changes");
  expect(bridge.closeEdit).toHaveBeenCalledWith("old");
  expect(host.querySelector("textarea")).toBeNull();
  expect(host.textContent).toContain("Kubernetes denied");
  expect(host.textContent).not.toContain("Review changes");
});

test("a request for a different Secret waits for explicit discard and never shows the old text", async () => {
  bridge.openEdit.mockReturnValueOnce({ promise: Promise.resolve({ target, id: "old", yaml: "data: original" }), cancel: vi.fn() });
  bridge.openEdit.mockReturnValueOnce({ promise: Promise.resolve({ target: other, id: "new", yaml: "data: selected" }), cancel: vi.fn() });
  function Harness() {
    const [editor, setEditor] = useState<EditorSession>(() => requestEditor(undefined, { target, tab: "a", reveal: false }));
    return (
      <>
        <button onClick={() => setEditor(requestEditor(editor, { target: other, tab: "b", reveal: true }))}>Reveal other</button>
        <EditDialog
          {...props}
          key={editor.tab}
          target={editor.target}
          connection={editor.target.connection}
          reveal={editor.reveal}
          suspended={editor.suspended && !editor.replacement}
          closing={!!editor.replacement}
          replacing={!!editor.replacement}
          onSuspend={() => setEditor({ ...editor, suspended: true })}
          onCancelClose={() => setEditor({ ...editor, replacement: undefined })}
          onClose={() => setEditor(requestEditor(undefined, editor.replacement!))}
        />
      </>
    );
  }
  await act(async () => root.render(<Harness />));
  await type("data: unsaved");
  await click("Keep draft");
  await click("Reveal other");
  expect(host.querySelector("textarea")).toBeNull();
  expect(bridge.openEdit).toHaveBeenCalledTimes(1);
  await click("Keep editing");
  await click("Reveal other");
  await click("Discard changes");
  expect(host.querySelector("textarea")?.value).toBe("data: selected");
  expect(bridge.openEdit).toHaveBeenLastCalledWith(other);
  expect(bridge.closeEdit).toHaveBeenCalledWith("old");
});

test("a retained generic view applies the accepted edit when discovery becomes ready", async () => {
  const descriptor = {
    group: "example.test",
    version: "v1",
    resource: "widgets",
    kind: "Widget",
    namespaced: true,
    handle: "handle",
    mode: "generic",
    reason: "",
    list: true,
    get: true,
    patch: true,
  };
  const resource = { ...target, group: descriptor.group, resource: descriptor.resource, kind: descriptor.kind };
  const capture = { resource, resourceVersion: "7", fetchedAt: "2026-11-16T05:00:00Z", objectJSON: "{}", yaml: "spec: before" };
  const accepted = {
    action: "edit",
    edit: resource,
    operationID: "save",
    state: "accepted",
    finishedAt: "2026-11-16T05:01:00Z",
  } as OperationResult;
  let resolveGroups!: (value: unknown) => void;
  bridge.groups.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveGroups = resolve;
    }),
  );
  bridge.resources.mockResolvedValueOnce({ resources: [descriptor], epoch: "version", fetchedAt: "" });
  bridge.inventory.mockResolvedValueOnce({ items: [], page: { nextCursor: "" } });
  bridge.detail.mockResolvedValueOnce({ ...capture, resourceVersion: "8", fetchedAt: "2026-11-16T05:02:00Z", yaml: "spec: after" });
  function Browser() {
    const [route, onRoute] = useState<CatalogSeed | undefined>({
      group: descriptor.group,
      version: "v1",
      namespace: resource.namespace,
      descriptor,
      capture,
      section: "YAML",
    });
    return (
      <APIResourceBrowser
        connection={resource.connection}
        namespace={resource.namespace}
        route={route}
        onRoute={(next) => onRoute(next ?? undefined)}
        operationResults={[accepted]}
        onEdit={vi.fn()}
        onBack={vi.fn()}
        onDedicated={vi.fn()}
        onResize={vi.fn()}
        onCommit={vi.fn()}
        widthPercent={40}
        heightPercent={40}
      />
    );
  }
  await act(async () => root.render(<Browser />));
  expect(bridge.detail).not.toHaveBeenCalled();
  await act(async () => resolveGroups({ groups: [{ group: descriptor.group, versions: ["v1"], preferred: "v1" }], epoch: "root" }));
  expect(bridge.detail).toHaveBeenCalledOnce();
  expect(host.querySelector("textarea")?.value).toBe("spec: after");
});
