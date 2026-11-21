// @vitest-environment happy-dom
/*
Purpose: Verify that refreshing the workload inventory does not abandon a valid rollback review.
Used by: Frontend operation regression tests.
Responsibilities: Exercise mounted capture boundaries and revocation.
Important: Native review calls are mocked; no cluster is used.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ResourceDetail } from "../workspace/ResourceDetail";
import { emptyKind } from "../workloads/state";
import type { WorkloadController } from "../workloads/useWorkloads";
import type { RolloutHistoryPage } from "../availability/model";
import type { RollbackRequest } from "./api";

const bridge = vi.hoisted(() => ({ prepareRollback: vi.fn(), revokeRollback: vi.fn() }));
vi.mock("./api", () => bridge);
vi.mock("@wailsio/runtime", () => ({}));

it("keeps the same review through an inventory tick, then revokes it on section refresh", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.body.appendChild(document.createElement("div"));
  const root = createRoot(host);
  const resource = {
    connection: { id: "a", generation: 1 },
    apiVersion: "apps/v1",
    kind: "StatefulSet",
    namespace: "fixture",
    name: "web",
    uid: "web",
  };
  const history: RolloutHistoryPage = {
    sourceVersion: "9",
    items: [
      {
        resource: { ...resource, kind: "ControllerRevision", name: "old", uid: "old" },
        resourceVersion: "7",
        revision: "1",
        createdAt: null,
        images: null,
        changeCause: "",
      },
    ],
    currentRevision: "old",
    updateRevision: "not-on-this-page",
    page: { nextCursor: "next", resourceVersion: "9", fetchedAt: "" },
    complete: false,
    problem: null,
  };
  bridge.prepareRollback.mockImplementation((request: RollbackRequest) => ({
    promise: Promise.resolve({
      request,
      token: "review",
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      beforeYAML: "new",
      proposedYAML: "old",
    }),
    cancel: vi.fn(),
  }));
  const render = (refresh: number, sectionRefresh: number) => {
    const controller = {
      selection: { resource },
      activeSection: "Revisions",
      sections: { Revisions: { data: history, loading: false, error: "" } },
      inventory: { ...emptyKind(), refresh, sectionRefresh },
    } as WorkloadController;
    return act(async () =>
      root.render(<ResourceDetail controller={controller} context="local" stale={false} canReviewOperation onOperation={vi.fn()} />),
    );
  };
  try {
    await render(0, 0);
    expect(host.textContent).toContain("Captured updateRevision: not-on-this-page");
    await act(async () => host.querySelector<HTMLInputElement>('input[type="radio"]')?.click());
    const review = [...host.querySelectorAll("button")].find((button) => button.textContent === "Roll back…");
    expect(review?.disabled).toBe(false);
    await act(async () => review?.click());
    expect(host.querySelector("dialog")?.textContent ?? "").toContain("Server dry-run passed.");
    await render(1, 0);
    expect(host.querySelector("dialog")?.textContent ?? "").toContain("Server dry-run passed.");
    expect(bridge.revokeRollback).not.toHaveBeenCalled();
    await render(1, 1);
    expect(host.querySelector("dialog")).toBeNull();
    expect(bridge.revokeRollback).toHaveBeenCalledExactlyOnceWith("review");
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});
