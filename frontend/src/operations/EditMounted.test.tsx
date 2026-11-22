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
import { applyEditorHidden, settleApplyEditor } from "./applyState";
import { requestEditor, type EditorDestination, type EditorSession } from "./editState";
import type { OperationResult } from "./api";

const bridge = vi.hoisted(() => ({
  closeEdit: vi.fn(),
  openApply: vi.fn(),
  prepareApply: vi.fn(),
  revokeApply: vi.fn(),
  closeApply: vi.fn(),
  openEdit: vi.fn(),
  prepareEdit: vi.fn(),
  revokeEdit: vi.fn(),
  groups: vi.fn(),
  resources: vi.fn(),
  inventory: vi.fn(),
  detail: vi.fn(),
  appliedDetail: vi.fn(),
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
    ReadAppliedDetail: bridge.appliedDetail,
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
    create: true,
    delete: false,
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

test.each(["accepted", "unknown", "detached"])("delayed apply %s settles only its submitted draft", async (state) => {
  let finish!: (accepted: boolean) => void;
  const response = new Promise<boolean>((resolve) => {
    finish = resolve;
  });
  const destination = { mode: "apply" as const, tab: "a", target, reveal: false as const, id: "apply-ui" };
  const draft = {
    id: "native-apply",
    target,
    resourceVersion: "3",
    cluster: "a",
    yaml: "kind: Secret",
    baselineYAML: "data: sensitive-fixture",
  };
  const preview = {
    target,
    token: "",
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
    beforeYAML: "unchanged",
    serverYAML: "unchanged",
    proposedYAML: "secret-proposal",
    force: false,
    ownershipIncomplete: false,
    ownershipTruncated: false,
    warnings: [],
  };
  bridge.openApply.mockReturnValue({ promise: Promise.resolve(draft), cancel: vi.fn() });
  bridge.prepareApply
    .mockReturnValueOnce({
      promise: Promise.resolve({ ...preview, conflicts: [{ field: ".data.marker", manager: "another-manager" }] }),
      cancel: vi.fn(),
    })
    .mockReturnValueOnce({ promise: Promise.resolve({ ...preview, token: "forced-review", force: true }), cancel: vi.fn() });

  function Harness() {
    const [editor, setEditor] = useState<EditorSession<EditorDestination> | undefined>(() => requestEditor(undefined, destination));
    const [origin, setOrigin] = useState(true);
    if (editor?.mode !== "apply") return null;
    return (
      <>
        <button disabled={editor.submitting} onClick={() => setEditor((current) => requestEditor(current, destination))}>
          Resume apply
        </button>
        <button onClick={() => setOrigin(false)}>Close origin tab</button>
        <EditDialog
          {...props}
          apply
          connection={origin ? props.connection : undefined}
          suspended={applyEditorHidden(editor.suspended, origin, origin)}
          onConfirm={(token, id, serverYAML) => {
            props.onConfirm(token, id, serverYAML);
            setEditor({ ...editor, submitting: true, suspended: true });
            void response.then((accepted) => setEditor((current) => settleApplyEditor(current, editor.id, accepted)));
          }}
        />
      </>
    );
  }
  await act(async () => root.render(<Harness />));
  const desired = "stringData: {marker: synthetic-pending}";
  await type(desired);
  await click("Review changes");
  expect(host.textContent).toContain("another-manager");
  expect(Array.from(host.querySelectorAll("button")).some((element) => element.textContent === "Confirm")).toBe(false);
  const takeover = host.querySelector<HTMLInputElement>("input[type=checkbox]")!;
  await act(async () => takeover.click());
  expect(bridge.prepareApply).toHaveBeenLastCalledWith({ draftID: draft.id, yaml: desired, takeOwnership: true });
  expect(host.textContent).toContain("No value changes");
  await click("Confirm");
  expect(props.onConfirm).toHaveBeenCalledExactlyOnceWith("forced-review", draft.id, "unchanged");
  expect(host.querySelector("textarea")).toBeNull();
  expect(host.querySelector<HTMLButtonElement>("button")?.disabled).toBe(true);
  expect(host.textContent).not.toContain("sensitive-fixture");
  if (state === "detached") await click("Close origin tab");
  await act(async () => finish(state === "accepted"));
  if (state === "detached") {
    await click("Resume apply");
    expect(host.querySelector("textarea")?.value).toBe(desired);
    expect(host.querySelector("textarea")?.readOnly).toBe(true);
    const review = Array.from(host.querySelectorAll("button")).find((button) => button.textContent === "Review changes");
    expect(review?.disabled).toBe(true);
    expect(host.textContent).toContain("You can copy the draft");
  } else if (state === "unknown") {
    await click("Resume apply");
    expect(host.querySelector("textarea")?.value).toBe(desired);
    expect(host.textContent).not.toContain("Take over conflicting fields");
    bridge.prepareApply.mockReturnValueOnce({ promise: Promise.resolve({ ...preview, token: "fresh" }), cancel: vi.fn() });
    await click("Review changes");
    expect(bridge.prepareApply).toHaveBeenLastCalledWith({ draftID: draft.id, yaml: desired, takeOwnership: false });
  } else {
    expect(host.querySelector("button")).toBeNull();
  }
  const newer = { ...requestEditor(undefined, { ...destination, id: "newer" }), submitting: true };
  expect(settleApplyEditor(newer, destination.id, true)).toBe(newer);
});

test("the accepted apply detail refreshes by operation ID when discovery does not advertise List", async () => {
  const descriptor = {
    group: "review.test",
    version: "v1beta1",
    resource: "widgets",
    kind: "Widget",
    namespaced: true,
    handle: "",
    mode: "unavailable",
    reason: "No List",
    list: false,
    get: true,
    patch: true,
    create: true,
    delete: false,
  };
  const resource = {
    ...target,
    group: descriptor.group,
    version: descriptor.version,
    resource: descriptor.resource,
    kind: descriptor.kind,
  };
  const capture = { resource, resourceVersion: "7", fetchedAt: "2026-11-17T09:00:00Z", objectJSON: "{}", yaml: "spec: before" };
  bridge.groups.mockResolvedValueOnce({ groups: [{ group: descriptor.group, versions: [descriptor.version] }], epoch: "root" });
  bridge.resources.mockResolvedValueOnce({ resources: [descriptor], epoch: "version" });
  bridge.appliedDetail.mockResolvedValueOnce({ ...capture, resourceVersion: "8", yaml: "spec: refreshed" });
  function Browser() {
    const [route, setRoute] = useState<CatalogSeed>({
      ...descriptor,
      namespace: resource.namespace,
      descriptor,
      capture,
      section: "YAML",
      createdDetailOperationID: "accepted-apply",
      detailFromApply: true,
    });
    return (
      <APIResourceBrowser
        connection={resource.connection}
        namespace={resource.namespace}
        route={route}
        onRoute={(next) => next && setRoute(next)}
        onBack={vi.fn()}
        onDedicated={vi.fn()}
        onApply={vi.fn()}
        operationResults={[]}
        onResize={vi.fn()}
        onCommit={vi.fn()}
        widthPercent={40}
        heightPercent={40}
      />
    );
  }
  await act(async () => root.render(<Browser />));
  await click("Refresh object");
  expect(bridge.appliedDetail).toHaveBeenCalledExactlyOnceWith("request", "accepted-apply");
  expect(bridge.inventory).not.toHaveBeenCalled();
  expect(host.querySelector("textarea")?.value).toBe("spec: refreshed");
});
