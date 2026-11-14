/*
Purpose: Present captured policy rules and aggregation selectors in separate groups.
Used by: Role and ClusterRole Overview.
Responsibilities: Page each array and mount nested content only while open.
Important: These fields describe recorded rules, not what a subject can do.
*/
import { useState, type ReactNode } from "react";
import { EvidencePage } from "../storage/StorageOverview";
import { recognizedRule, type Aggregation, type Rule } from "./model";

export function Disclosure({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <details
      onToggle={(event) => {
        if (event.target === event.currentTarget) setOpen(event.currentTarget.open);
      }}
    >
      <summary>{label}</summary>
      {open && children}
    </details>
  );
}

export function Field({ label, children }: { label: string; children?: ReactNode }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className="network-value">{children ?? "Not reported"}</dd>
    </div>
  );
}

function Values({ items, empty = "None captured", core = false }: { items?: string[]; empty?: string; core?: boolean }) {
  if (!items?.length) return empty;
  return (
    <ul>
      <EvidencePage
        items={items}
        render={(value, i) => (
          <li key={i} className="network-value">
            {value === "*" ? "* (wildcard)" : core && value === "" ? "Core API group" : value || '""'}
          </li>
        )}
      />
    </ul>
  );
}

export function PolicyRules({
  rules = [],
  aggregationRule,
  namespaced,
}: {
  rules?: Rule[];
  aggregationRule?: Aggregation;
  namespaced: boolean;
}) {
  const selectors = aggregationRule?.clusterRoleSelectors ?? [];
  return (
    <>
      {aggregationRule && (
        <section>
          <h3>Aggregation selectors · {selectors.length} captured</h3>
          <p className="muted">
            Selectors are alternatives. Requirements inside each selector stay grouped. Captured rules and selectors may reflect different
            reconciliation times.
          </p>
          {!selectors.length && <p>No selectors captured.</p>}
          <EvidencePage
            items={selectors}
            render={(selector, i) => (
              <Disclosure key={i} label={`Selector ${i + 1}`}>
                <h4>Match labels</h4>
                <ul>
                  <EvidencePage
                    items={Object.entries(selector.matchLabels ?? {})}
                    render={([key, value]) => (
                      <li className="network-value" key={key}>
                        {key} = {value === "" ? '""' : value}
                      </li>
                    )}
                  />
                </ul>
                <h4>Match expressions</h4>
                <EvidencePage
                  items={selector.matchExpressions ?? []}
                  render={(expression, index) => (
                    <Disclosure key={index} label={`Requirement ${index + 1}`}>
                      <dl className="pod-facts">
                        <Field label="Key">{expression.key}</Field>
                        <Field label="Operator">{expression.operator}</Field>
                        <Field label="Values">
                          <Values items={expression.values} />
                        </Field>
                      </dl>
                    </Disclosure>
                  )}
                />
              </Disclosure>
            )}
          />
        </section>
      )}
      <h3>
        {aggregationRule ? "Captured controller-managed rules" : "Policy rules"} · {rules.length} captured
      </h3>
      <p className="muted">
        RBAC grants are additive. Rules describe configuration. Non-resource URL rules apply through ClusterRoleBindings.
      </p>
      {!rules.length && <p>No rules captured.</p>}
      <EvidencePage
        items={rules}
        render={(rule, i) => (
          <Disclosure key={i} label={`Rule ${i + 1}`}>
            {(!recognizedRule(rule) || (namespaced && !!rule.nonResourceURLs?.length)) && <p>Unrecognized rule shape</p>}
            <dl className="pod-facts">
              <Field label="Verbs">
                <Values items={rule.verbs} />
              </Field>
              <Field label="API groups">
                <Values items={rule.apiGroups} core />
              </Field>
              <Field label="Resources / subresources">
                <Values items={rule.resources} />
              </Field>
              <Field label="Resource names">
                <Values items={rule.resourceNames} empty="No name restriction in this rule" />
              </Field>
              <Field label="Non-resource URLs">
                <Values items={rule.nonResourceURLs} />
              </Field>
            </dl>
          </Disclosure>
        )}
      />
    </>
  );
}
