// @vitest-environment happy-dom
/*
Purpose: Verify explicit named lookup and reviews against the mounted Projects workspace.
Used by: Frontend Project regressions.
Responsibilities: Separate inspection from switching, revoke cancelled previews and require typed deletion.
Important: Uses synthetic Projects and a mock bridge.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { test, vi, expect } from "vitest";
import { ProjectsWorkspace } from "./ProjectsWorkspace";
const api = vi.hoisted(() => ({
  ReadProject: vi.fn(),
  ProjectQuotas: vi.fn(),
  MyProjectAccess: vi.fn(),
  PrepareDelete: vi.fn(),
  RevokeDelete: vi.fn(),
  PrepareProjectRequest: vi.fn(),
  RevokeProjectRequest: vi.fn(),
}));
vi.mock("../connections/api", () => ({ api, message: String }));
test("explicit lookup, scope selection and revocable, typed review", async () => {
  const connection = { id: "one", generation: 1 };
  const resource = { connection, apiVersion: "project.openshift.io/v1", kind: "Project", namespace: "", name: "team", uid: "first" };
  const project = {
    resource,
    version: "7",
    displayName: "Team",
    description: "Example",
    truncated: false,
    phase: "Active",
    createdAt: "",
    deletingAt: "",
  };
  api.ReadProject.mockResolvedValue(project);
  api.PrepareDelete.mockResolvedValue({ cluster: "cluster-a", token: "review" });
  const submit = vi.fn(),
    useProject = vi.fn();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const click = async (text: string) => {
    const btn = [...host.querySelectorAll("button")].find((b) => b.textContent === text);
    expect(btn).toBeDefined();
    await act(async () => btn!.click());
  };
  const input = async (el: HTMLInputElement, value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(el, value);
      el.dispatchEvent(new Event("input", { bubbles: true }));
    });
  };
  try {
    await act(async () =>
      root.render(
        <ProjectsWorkspace
          connection={connection}
          caps={{ projects: true, list: false, get: true, request: true, checkRequest: true, delete: true }}
          items={[]}
          partial
          error="List forbidden"
          refresh={vi.fn()}
          useProject={useProject}
          submit={submit}
          split={{ widthPercent: 45, heightPercent: 45, onResize: vi.fn(), onCommit: vi.fn() }}
        />,
      ),
    );
    await input(host.querySelector("input")!, "team");
    expect(api.ReadProject).not.toHaveBeenCalled();
    await act(async () => {
      host.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });
    expect(api.ReadProject).toHaveBeenCalledWith(expect.any(String), connection, "team", "");
    expect(useProject).not.toHaveBeenCalled();
    await click("Use project");
    expect(useProject.mock.calls[0][0].resource.uid).toBe("first");
    await click("Delete project");
    await click("Review deletion");
    expect(api.PrepareDelete).toHaveBeenCalledWith(expect.any(String), { resource, sourceVersion: "7" });
    expect([...host.querySelectorAll("dialog button")].find((b) => b.textContent === "Confirm deletion")?.hasAttribute("disabled")).toBe(
      true,
    );
    await click("Cancel");
    expect(api.RevokeDelete).toHaveBeenCalledWith("review");
    expect(submit).not.toHaveBeenCalled();
    let loseA!: (e: Error) => void;
    api.ProjectQuotas.mockReturnValueOnce(new Promise((_, reject) => (loseA = reject)));
    await click("Read quotas");
    api.ReadProject.mockResolvedValueOnce({ ...project, resource: { ...resource, name: "team-b", uid: "second" } });
    await input(host.querySelector("input")!, "team-b");
    await act(async () => {
      host.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
    });
    await act(async () => loseA(new Error("project_unavailable")));
    expect(host.querySelector(".detail-pane h2")?.textContent).toBe("team-b");
    let loseB!: (e: Error) => void;
    api.MyProjectAccess.mockReturnValueOnce(new Promise((_, reject) => (loseB = reject)));
    await click("Check my access");
    await click("Delete project");
    await click("Review deletion");
    await act(async () => loseB(new Error("project_unavailable")));
    expect(host.querySelector("dialog")).toBeNull();
    expect(host.querySelector(".detail-pane h2")).toBeNull();
    expect(api.RevokeDelete).toHaveBeenCalledTimes(2);
    expect(api.PrepareProjectRequest).not.toHaveBeenCalled();
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});

test("Project picker filters locally and verifies even a selected same-name Project", async () => {
  const { NamespaceControl } = await import("../workspace/NamespaceControl");
  const root = createRoot(document.body.appendChild(document.createElement("div")));
  const select = vi.fn();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  try {
    await act(async () =>
      root.render(<NamespaceControl project namespace="configured" names={["team"]} complete={false} disabled={false} onSelect={select} />),
    );
    const trigger = document.querySelector<HTMLButtonElement>(".namespace-trigger")!;
    expect(trigger.textContent).toContain("(unverified)");
    await act(async () => trigger.click());
    const options = [...document.querySelectorAll('[role="option"]')];
    expect(options.map((o) => o.textContent)).toEqual(["team"]);
    const input = document.querySelector<HTMLInputElement>('[role="combobox"]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "configured");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(select).not.toHaveBeenCalled();
    await act(async () => {
      input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(select).toHaveBeenCalledWith("configured");
  } finally {
    await act(async () => root.unmount());
    vi.unstubAllGlobals();
  }
});
