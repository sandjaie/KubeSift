/*
Purpose: Protect synthetic workspace target isolation.
Used by: Vitest checks.
Responsibilities: Exercise independent tabs, frozen reviews and session ownership.
Important: All resources are synthetic; no desktop bridge is imported.
*/
import { expect, it } from "vitest";
import { createPreview } from "../preview/fixtures";
import { initialUiState, reduceUi } from "./state";
it("keeps tab navigation and review targets isolated", () => {
  const preview = createPreview();
  const [a, b] = preview.tabs;
  const ra = preview.resources.find((r) => r.key.clusterId === a.id)!;
  const rb = preview.resources.find((r) => r.key.clusterId === b.id)!;
  let s = reduceUi(initialUiState(), { type: "preview", ...preview });
  s = reduceUi(s, {
    type: "tab",
    id: a.id,
    patch: {
      scope: { kind: "namespace", name: ra.key.namespace! },
      selected: ra.key,
      filter: "checkout",
      detail: "logs",
      scroll: { list: 32, detail: 64 },
    },
  });
  s = reduceUi(s, { type: "review", action: "delete", target: ra.key });
  const original = s.tabs[0];
  const review = s.review!;
  s = reduceUi(s, { type: "selectTab", id: b.id });
  s = reduceUi(s, {
    type: "tab",
    id: b.id,
    patch: {
      scope: { kind: "namespace", name: rb.key.namespace! },
      selected: rb.key,
      filter: "api",
      detail: "yaml",
      scroll: { list: 9, detail: 8 },
    },
  });
  expect(s.tabs[0]).toEqual(original);
  expect(s.review?.target).toEqual(ra.key);
  s = reduceUi(s, { type: "closeTab", id: a.id });
  s = reduceUi(s, { type: "confirmReview", id: review.id });
  expect(s.resources.some((r) => r.key.uid === rb.key.uid)).toBe(true);
  expect(s.review).toBeNull();
});
it("preserves sessions while hidden and closes only the confirmed owner", () => {
  const p = createPreview();
  const [a, b] = p.tabs;
  let s = reduceUi(initialUiState(), { type: "preview", ...p });
  const ra = p.resources.find((r) => r.key.clusterId === a.id)!;
  const rb = p.resources.find((r) => r.key.clusterId === b.id)!;
  s = reduceUi(s, { type: "startSession", kind: "terminal", target: ra.key });
  s = reduceUi(s, { type: "startSession", kind: "logs", target: rb.key });
  const sessions = s.sessions;
  expect(sessions).toHaveLength(2);
  s = reduceUi(s, { type: "dock", open: false });
  s = reduceUi(s, { type: "selectTab", id: b.id });
  expect(s.sessions).toEqual(sessions);
  s = reduceUi(s, { type: "closeTab", id: a.id });
  expect(s.tabs).toHaveLength(2);
  s = reduceUi(s, { type: "cancelClose" });
  expect(s.sessions).toEqual(sessions);
  s = reduceUi(s, { type: "closeTab", id: a.id });
  s = reduceUi(s, { type: "confirmClose", id: a.id });
  expect(s.sessions).toEqual([sessions[1]]);
  s = reduceUi(s, { type: "stopSession", id: sessions[0].id });
  expect(s.sessions).toEqual([sessions[1]]);
  s = reduceUi(s, { type: "stopSession", id: sessions[1].id });
  expect(s.sessions[0].status).toBe("stopped");
});
it("keeps investigation evidence bound across navigation and explicit reopening", () => {
  const p = createPreview();
  let s = reduceUi(initialUiState(), { type: "preview", ...p });
  const resource = p.resources[0];
  s = reduceUi(s, { type: "investigate", target: resource.key });
  const investigation = s.investigation;
  expect(investigation?.target).toEqual(resource.key);
  expect(investigation?.evidenceIds.length).toBeGreaterThan(0);
  s = reduceUi(s, { type: "selectTab", id: p.tabs[1].id });
  s = reduceUi(s, { type: "page", page: "settings" });
  s = reduceUi(s, { type: "ai", open: false });
  s = reduceUi(s, { type: "ai", open: true });
  expect(s.investigation).toEqual(investigation);
  s = reduceUi(s, { type: "investigate", target: null });
  s = reduceUi(s, { type: "selectTab", id: p.tabs[0].id });
  expect(s.investigation?.target).toBeNull();
});
