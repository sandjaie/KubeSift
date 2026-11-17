// @vitest-environment happy-dom
/*
Purpose: Verify mounted creation review, draft retention and result navigation.
Used by: Frontend operation regression tests.
Responsibilities: Exercise scope and generation changes, late tokens and one confirmation.
Important: The bridge uses synthetic data and never connects to Kubernetes.
*/
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { CreateDialog } from "./CreateDialog";
import { OperationResults, useOperationResults } from "./OperationResults";
import { requestEditor, settleCreateEditor, type EditorDestination, type EditorSession } from "./editState";
import type { CreatePreview, OperationResult } from "./api";

const bridge = vi.hoisted(() => ({
  openCreate: vi.fn(),
  closeCreate: vi.fn(),
  prepareCreate: vi.fn(),
  revokeCreate: vi.fn(),
  inspectCreateName: vi.fn(),
  executeCreate: vi.fn(),
  executeDelete: vi.fn(),
  executeEdit: vi.fn(),
  executeApply: vi.fn(),
  executeRestart: vi.fn(),
  executeRollback: vi.fn(),
  executeScale: vi.fn(),
  listOperationResults: vi.fn(),
  checkDeleteTarget: vi.fn(),
}));
vi.mock("./api", () => bridge);
vi.mock("@wailsio/runtime", () => ({}));

const connection = { id: "cluster", generation: 1 };
const destination = { mode: "create" as const, id: "local-editor", tab: "a", connection, namespace: "fixture" };
const target = { connection, group: "", version: "v1", resource: "secrets", kind: "Secret", namespace: "fixture", name: "demo" };
const preview: CreatePreview = {
  token: "review",
  target,
  expiresAt: new Date(Date.now() + 300000).toISOString(),
  proposedYAML: "data: synthetic-proposal",
  serverYAML: "data: synthetic-preview",
  warnings: [],
};
const onConfirm = vi.fn();
const onClose = vi.fn();
const props = {
  destination,
  context: "a",
  connection,
  suspended: false,
  closing: false,
  replacing: false,
  onSuspend: vi.fn(),
  onCancelClose: vi.fn(),
  onClose,
  onConfirm,
};
let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.clearAllMocks();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  bridge.openCreate.mockReturnValue({
    promise: Promise.resolve({ id: "native-draft", connection, namespace: "fixture", cluster: "a" }),
    cancel: vi.fn(),
  });
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});
const button = (name: string) => Array.from(host.querySelectorAll("button")).find((b) => b.textContent === name)!;
async function click(name: string) {
  expect(button(name), name).toBeDefined();
  await act(async () => button(name).click());
}
async function type(value: string) {
  const field = host.querySelector("textarea")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

test("review shows the exact scope and confirms only the opaque token without copying", async () => {
  const copy = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
  bridge.prepareCreate.mockReturnValueOnce({ promise: Promise.resolve(preview), cancel: vi.fn() });
  await act(async () => root.render(<CreateDialog {...props} />));
  await type("apiVersion: v1\nkind: Secret\nmetadata: {name: demo}\nstringData: {key: synthetic}");
  await click("Review create");
  expect(bridge.prepareCreate.mock.calls[0][0]).toMatchObject({ draftID: "native-draft", namespace: "fixture", resource: "" });
  expect(host.textContent).toContain("synthetic-proposal");
  expect(host.textContent).toContain("synthetic-preview");
  expect(host.textContent).toContain("v1 · secrets · demo");
  const confirm = button("Confirm create");
  await act(async () => {
    confirm.click();
    confirm.click();
  });
  expect(onConfirm).toHaveBeenCalledTimes(1);
  expect(onConfirm).toHaveBeenCalledWith("review");
  expect(copy).not.toHaveBeenCalled();
  copy.mockRestore();
});

test("suspension retains typed text and a changed generation cancels preparation and revokes its late token", async () => {
  let resolve!: (value: CreatePreview) => void;
  const cancel = vi.fn();
  bridge.prepareCreate.mockReturnValueOnce({
    promise: new Promise<CreatePreview>((done) => {
      resolve = done;
    }),
    cancel,
  });
  await act(async () => root.render(<CreateDialog {...props} />));
  await type("stringData: {key: synthetic-unsaved}");
  await click("Review create");
  await act(async () => root.render(<CreateDialog {...props} suspended />));
  expect(cancel).toHaveBeenCalled();
  expect(host.textContent).not.toContain("synthetic-unsaved");
  await act(async () => resolve(preview));
  expect(bridge.revokeCreate).toHaveBeenCalledWith("review");
  await act(async () => root.render(<CreateDialog {...props} connection={{ ...connection, generation: 2 }} />));
  expect(host.querySelector("textarea")?.value).toContain("synthetic-unsaved");
  expect(button("Review create").disabled).toBe(true);
  expect(bridge.closeCreate).toHaveBeenCalledWith("native-draft");
  await click("Cancel");
  expect(host.textContent).toContain("Discard unsaved YAML changes");
  await click("Discard changes");
  expect(onClose).toHaveBeenCalled();
});

test("a create draft waits for explicit replacement when an edit is requested", () => {
  const current = requestEditor(undefined, destination);
  const next: EditorDestination = { tab: "a", target: { ...target, uid: "created" }, reveal: false };
  const decision = requestEditor(current, next);
  expect(decision.mode).toBe("create");
  expect(decision.replacement).toEqual(next);
});

test.each(["accepted", "unknown"])("a delayed %s submission owns its draft until the outcome arrives", async (state) => {
  let finish!: (result: { state: string }) => void;
  bridge.prepareCreate.mockReturnValueOnce({ promise: Promise.resolve(preview), cancel: vi.fn() });
  bridge.listOperationResults.mockResolvedValue([]);
  bridge.executeCreate.mockReturnValueOnce(new Promise((resolve) => (finish = resolve)));
  function Harness() {
    const [editor, setEditor] = useState<EditorSession<EditorDestination> | undefined>(() => requestEditor(undefined, destination));
    const { submit } = useOperationResults(() => {});
    if (editor?.mode !== "create") return null;
    return (
      <>
        <button disabled={editor.submitting} onClick={() => setEditor((current) => requestEditor(current, destination))}>
          Resume create draft
        </button>
        <button onClick={() => setEditor((current) => requestEditor(current, destination))}>Request same editor</button>
        <CreateDialog
          {...props}
          suspended={editor.suspended}
          onConfirm={(token) => {
            setEditor({ ...editor, submitting: true, suspended: true });
            void submit("create", token, "").then((result) => {
              setEditor((current) => settleCreateEditor(current, editor.id, result?.state === "accepted"));
            });
          }}
        />
      </>
    );
  }
  await act(async () => root.render(<Harness />));
  await type("stringData: {key: synthetic-pending}");
  await click("Review create");
  await click("Confirm create");
  expect(button("Resume create draft").disabled).toBe(true);
  await click("Request same editor");
  expect(host.querySelector("textarea")).toBeNull();
  await act(async () => finish({ state }));
  if (state === "accepted") {
    expect(button("Resume create draft")).toBeUndefined();
    expect(host.querySelector("textarea")).toBeNull();
  } else {
    await click("Resume create draft");
    expect(host.querySelector("textarea")?.value).toContain("synthetic-pending");
  }
  expect(bridge.executeCreate).toHaveBeenCalledTimes(1);
});

test("Unknown offers name inspection only, while accepted opening uses the actual UID", async () => {
  const ref = { connection, apiVersion: "v1", kind: "Secret", namespace: "fixture", name: "demo", uid: "" };
  const base = {
    action: "create",
    resource: ref,
    cluster: "a",
    startedAt: "",
    finishedAt: "2026-11-16T00:00:00Z",
    create: { target, resourceVersion: "" },
    problem: null,
    scale: null,
    restart: null,
    delete: null,
  };
  const results: OperationResult[] = [
    { ...base, operationID: "uncertain", state: "unknown" },
    { ...base, operationID: "created", state: "accepted", resource: { ...ref, uid: "actual" }, create: { target, resourceVersion: "71" } },
  ];
  const onTarget = vi.fn();
  const read = vi.fn();
  bridge.inspectCreateName.mockReturnValueOnce({
    promise: Promise.resolve({ operationID: "uncertain", checkedAt: base.finishedAt, state: "present", problem: null }),
    cancel: read,
  });
  await act(async () =>
    root.render(
      <OperationResults
        results={results}
        pending={0}
        error=""
        onClose={() => {}}
        onRefresh={() => {}}
        onTarget={onTarget}
        canOpen={() => true}
      />,
    ),
  );
  await click("Inspect name");
  expect(bridge.inspectCreateName).toHaveBeenCalledWith("uncertain");
  expect(host.textContent).toContain("does not establish");
  expect(bridge.executeCreate).not.toHaveBeenCalled();
  await click("Open resource");
  expect(onTarget).toHaveBeenCalledWith({ ...ref, uid: "actual" }, { target, operationID: "created" });
});
