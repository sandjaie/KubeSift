/*
Purpose: Render configured availability policy and reported observations.
Used by: HPA, PDB and ControllerRevision Overview.
Responsibilities: Keep metrics separate, lazy-page nested evidence and resolve explicit references.
Important: These captures do not authorize scaling, eviction or rollback.
*/
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { Disclosure, Field } from "../access-control/PolicyRules";
import { EvidencePage } from "../storage/StorageOverview";
import { evidenceTime } from "../workspace/ResourceOverview";
import { availabilityTarget, observationLabel, selectorLabel, type Reference } from "./model";

type Fields = Record<string, unknown>;
type Metric = Fields & { type?: string };
type Capture = {
  spec?: Fields & { metrics?: Metric[]; scaleTargetRef?: Reference };
  status?: Fields & { currentMetrics?: Metric[] };
  metadata?: Fields & { ownerReferences?: (Reference | null)[] };
  data?: unknown;
};

function Value({ value }: { value: unknown }) {
  if (value == null) return "Not reported";
  if (typeof value !== "object") return value === "" ? '""' : String(value);
  const entries = Object.entries(value);
  if (!entries.length) return "None captured";
  return (
    <Disclosure label={`Show ${entries.length} ${entries.length === 1 ? "entry" : "entries"}`}>
      <dl className="pod-facts">
        <EvidencePage
          items={entries}
          render={([key, item]) => (
            <Field key={key} label={key}>
              <Value value={item} />
            </Field>
          )}
        />
      </dl>
    </Disclosure>
  );
}

function Facts({ data, fields }: { data: Fields; fields: [string, string][] }) {
  return (
    <dl className="pod-facts">
      {fields.map(([key, label]) => (
        <Field key={key} label={label}>
          <Value value={data[key]} />
        </Field>
      ))}
    </dl>
  );
}

const knownMetrics = ["Resource", "ContainerResource", "Pods", "Object", "External"];

function Metrics({ title, items }: { title: string; items?: Metric[] }) {
  const metrics = items ?? [];
  return (
    <section>
      <h3>{title}</h3>
      {!metrics.length && <p>Not reported</p>}
      <EvidencePage
        items={metrics}
        render={(metric, index) => (
          <Disclosure label={`${index + 1} · ${metric?.type || "Type not reported"}`} key={index}>
            {!knownMetrics.includes(metric?.type ?? "") && <p>Unrecognized metric type; captured fields are shown as configuration.</p>}
            <dl className="pod-facts">
              <EvidencePage
                items={Object.entries(metric ?? {}).filter(([key]) => key !== "type")}
                render={([key, value]) => (
                  <Field key={key} label={key}>
                    <Value value={value} />
                  </Field>
                )}
              />
            </dl>
          </Disclosure>
        )}
      />
    </section>
  );
}

export function AvailabilityOverview({ data, onOpenReference }: { data: ResourceDetail; onOpenReference: (target: ResourceRef) => void }) {
  const { resource } = data;
  const body = JSON.parse(data.objectJSON) as Capture;
  const spec = body.spec ?? {},
    status = body.status ?? {};
  const owners = body.metadata?.ownerReferences ?? [];
  const controlling = owners.filter((owner): owner is Reference => owner?.controller === true);
  const reference =
    resource.kind === "HorizontalPodAutoscaler" ? spec.scaleTargetRef : controlling.length === 1 ? controlling[0] : undefined;
  const target = availabilityTarget(resource, reference);
  return (
    <div className="evidence-section workload-overview" key={JSON.stringify([resource.uid, data.resourceVersion])}>
      {resource.kind === "ControllerRevision" ? (
        <>
          <p className="muted">Retained controller history is incomplete. Data is opaque historical content and may be a patch.</p>
          <dl className="pod-facts">
            <Field label="Revision">{data.availability?.revision || "Not reported"}</Field>
            <Field label="Controller owner">
              {target ? (
                <button className="network-value" onClick={() => onOpenReference(target)}>
                  {target.kind} {target.name}
                </button>
              ) : controlling.length > 1 ? (
                "Ambiguous controller owner"
              ) : reference ? (
                `${reference.kind ?? ""} ${reference.name ?? ""} · Reference unavailable`
              ) : (
                "Not reported"
              )}
            </Field>
            <Field label="Captured owner references">
              <Value value={owners} />
            </Field>
            <Field label="Metadata">
              <Value value={body.metadata} />
            </Field>
          </dl>
          <Disclosure label="Historical data">
            <Value value={body.data} />
          </Disclosure>
        </>
      ) : (
        <>
          <p className={data.availability?.observation === "stale" ? "warning" : "muted"}>
            {observationLabel(data.availability?.observation)}
          </p>
          {resource.kind === "HorizontalPodAutoscaler" ? (
            <>
              <p className="muted">
                Controller-reported observations are snapshots. Metric targets and observations are listed separately.
              </p>
              <dl className="pod-facts">
                <Field label="Scale target">
                  {target ? (
                    <button className="network-value" onClick={() => onOpenReference(target)}>
                      {target.kind} {target.name}
                    </button>
                  ) : reference ? (
                    `${reference.kind ?? ""} ${reference.name ?? ""} · Reference unavailable`
                  ) : (
                    "Not reported"
                  )}
                </Field>
                <Field label="Target API">{reference?.apiVersion}</Field>
              </dl>
              <Facts
                data={spec}
                fields={[
                  ["minReplicas", "Minimum replicas"],
                  ["maxReplicas", "Maximum replicas"],
                  ["behavior", "Scaling behavior"],
                ]}
              />
              <Facts
                data={status}
                fields={[
                  ["currentReplicas", "Reported current replicas"],
                  ["desiredReplicas", "Reported desired replicas"],
                  ["lastScaleTime", "Last scale time"],
                ]}
              />
              <Metrics title="Metric targets" items={spec.metrics} />
              <Metrics title="Reported metrics" items={status.currentMetrics} />
            </>
          ) : (
            <>
              <p className="muted">Disruptions allowed is a controller-reported snapshot. It does not establish eviction eligibility.</p>
              <dl className="pod-facts">
                <Field label="Selector">
                  {selectorLabel(spec.selector)}
                  {spec.selector != null && <Value value={spec.selector} />}
                </Field>
              </dl>
              <Facts
                data={spec}
                fields={[
                  ["minAvailable", "Minimum available"],
                  ["maxUnavailable", "Maximum unavailable"],
                  ["unhealthyPodEvictionPolicy", "Unhealthy Pod eviction policy"],
                ]}
              />
              <Facts
                data={status}
                fields={[
                  ["currentHealthy", "Reported current healthy"],
                  ["desiredHealthy", "Reported desired healthy"],
                  ["expectedPods", "Reported expected Pods"],
                  ["disruptionsAllowed", "Reported disruptions allowed"],
                  ["disruptedPods", "Disrupted Pods bookkeeping"],
                ]}
              />
            </>
          )}
          <Facts
            data={{
              conditions: status.conditions,
              generation: data.availability?.generation,
              observedGeneration: data.availability?.observedGeneration,
            }}
            fields={[
              ["generation", "Current generation"],
              ["observedGeneration", "Observed generation"],
              ["conditions", "Conditions"],
            ]}
          />
        </>
      )}
      <p className="muted">
        Captured {evidenceTime(data.fetchedAt)} · resourceVersion {data.resourceVersion || "Unknown"}
      </p>
    </div>
  );
}
