/*
Purpose: Check access-control evidence and lazy nested rendering.
Used by: Explorer frontend tests.
Responsibilities: Verify optional settings, inert references and scope semantics.
Important: Synthetic strings do not represent actual permissions.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import type { ResourceDetail } from "../workloads/model";
import { AccessControlOverview } from "./AccessControlOverview";
import { PodIdentity } from "./PodIdentity";
import { roleTarget, subjectTarget, recognizedRule } from "./model";

function capture(kind: string, value: unknown): ResourceDetail {
  return {
    resource: { connection: { id: "test", generation: 1 }, uid: "uid", name: "sample", namespace: "one", kind, apiVersion: "v1" },
    fetchedAt: "",
    resourceVersion: "7",
    job: null,
    objectJSON: JSON.stringify(value),
  };
}

test("binding subject scope is independent of role scope and preserves duplicate entries", () => {
  const data = capture("RoleBinding", {
    roleRef: { kind: "ClusterRole", name: "system:Fixture", apiGroup: "rbac.authorization.k8s.io" },
    subjects: [
      { kind: "ServiceAccount", name: "same" },
      { kind: "ServiceAccount", name: "same" },
      { kind: "User", name: "<script>alert(1)</script>" },
    ],
  });
  const markup = renderToStaticMarkup(<AccessControlOverview data={data} onOpenReference={() => {}} />);
  expect(markup).toContain("system:Fixture");
  expect(markup).toContain("Binding namespace");
  expect(markup.match(/>same<\/button>/g)).toHaveLength(2);
  expect(markup).toContain("&lt;script&gt;");
  expect(markup).toContain("within this RoleBinding’s namespace");
  expect(roleTarget(data.resource, { kind: "Role", name: "bad/path", apiGroup: "rbac.authorization.k8s.io" })).toBeUndefined();
  expect(
    subjectTarget({ ...data.resource, kind: "ClusterRoleBinding", namespace: "" }, { kind: "ServiceAccount", name: "same" }),
  ).toBeUndefined();
  expect(subjectTarget(data.resource, { kind: "ServiceAccount", name: "same", namespace: "other" })?.namespace).toBe("other");
  expect(subjectTarget(data.resource, { kind: "Group", name: "system:serviceaccount:one:same" })).toBeUndefined();
});

test("rules stay separate and collapsed nested rule and selector lists stay unmounted", () => {
  const verbs = Array.from({ length: 150 }, () => "nested-sentinel");
  const data = capture("ClusterRole", {
    rules: Array.from({ length: 101 }, () => ({ verbs, apiGroups: [""], resources: ["pods/log"] })),
    aggregationRule: {
      clusterRoleSelectors: [{ matchLabels: { unique: "selector-sentinel" } }, { matchExpressions: [{ key: "x", operator: "Exists" }] }],
    },
  });
  const markup = renderToStaticMarkup(<AccessControlOverview data={data} onOpenReference={() => {}} />);
  expect(markup).toContain("Rule 100");
  expect(markup).not.toContain("Rule 101");
  expect(markup).not.toContain("nested-sentinel");
  expect(markup).not.toContain("selector-sentinel");
  expect(markup).toContain("Selector 2");
  expect(recognizedRule({ verbs: ["*"], apiGroups: [""], resources: ["pods/log"] })).toBe(true);
  expect(recognizedRule({ verbs: ["get"], resources: ["pods"], nonResourceURLs: ["/healthz"] })).toBe(false);
});

test("Pod and ServiceAccount evidence never turn token configuration or Secret names into links", () => {
  const pod = capture("Pod", { spec: { serviceAccount: "obsolete", automountServiceAccountToken: false } });
  const markup = renderToStaticMarkup(<PodIdentity data={pod} onOpenReference={() => {}} />);
  expect(markup).toContain(">false<");
  expect(markup).not.toContain("obsolete");
  expect(markup).not.toContain("<button");
  const sa = capture("ServiceAccount", { secrets: [{ name: "inert-secret" }], imagePullSecrets: [{ name: "inert-image" }] });
  const view = renderToStaticMarkup(<AccessControlOverview data={sa} onOpenReference={() => {}} />);
  expect(view).toContain("Not specified");
  expect(view).not.toContain("<button");
});
