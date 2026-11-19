/*
Purpose: Present current usage without confusing missing, partial or old samples with zero.
Used by: Pod/Node tables and their Overview panels.
Responsibilities: Format millicores, binary memory and explicitly labelled configured comparisons.
Important: Pod subtotals never acquire a percentage; configuration is a later separate read.
*/
import type { MetricValue, ResourceRef } from "../../bindings/internal/kube/models";
import { metricsIdentity, type SampleView } from "./useMetrics";
export function metricText(amount: string | null, metric: "cpu" | "memory"): string {
  if (amount === null) return "Unavailable";
  if (metric === "cpu") return `${amount}m`;
  const [integer] = amount.split(".");
  const bytes = BigInt(integer);
  if (bytes === 0n && Number(amount) > 0) return `${amount} B`;
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"];
  let unit = 0,
    divisor = 1n;
  while (unit < units.length - 1 && bytes >= divisor * 1024n) {
    unit++;
    divisor *= 1024n;
  }
  const hundredths = (bytes * 100n + divisor / 2n) / divisor;
  const fraction = String(hundredths % 100n)
    .padStart(2, "0")
    .replace(/0+$/, "");
  return `${hundredths / 100n}${fraction ? `.${fraction}` : ""} ${units[unit]}`;
}
function labels(view: SampleView) {
  const age = view.sample.sampledAt ? Math.max(0, Math.floor((view.now - Date.parse(view.sample.sampledAt)) / 1000)) : 0;
  return { age, stale: !!view.sample.sampledAt && view.now - Date.parse(view.sample.sampledAt) > 60000 };
}
export function MetricsCell({ view, metric }: { view?: SampleView; metric: "cpu" | "memory" }) {
  if (!view) return <span className="muted">Unavailable</span>;
  const s = view.sample,
    { age, stale } = labels(view),
    complete = metric === "cpu" ? s.cpuComplete : s.memoryComplete;
  return (
    <span title={[s.apiVersion, s.problem?.message, view.last, `${age}s old`].filter(Boolean).join(" · ")}>
      {metricText(s[metric].amount, metric)}
      {s.resource.kind === "Node" && s[metric].amount !== null && (
        <span className="muted">
          {" "}
          ·{" "}
          {s[metric].allocatablePercent === null ? "Allocatable percentage unavailable" : `${s[metric].allocatablePercent}% of allocatable`}
        </span>
      )}
      {s[metric].amount !== null && !complete ? " · Partial subtotal" : ""}
      {view.last ? " · Last sample" : ""}
      {stale ? " · Stale" : ""}
    </span>
  );
}
function Comparisons({ value, metric, node = false }: { value: MetricValue; metric: "cpu" | "memory"; node?: boolean }) {
  return (
    <>
      {(node ? (["allocatable"] as const) : (["request", "limit"] as const)).map((key) => {
        const percent = value[`${key}Percent`];
        return (
          <p className="muted" key={key}>
            {metric === "cpu" ? "CPU" : "Memory"} {key}: {value[key] === null ? "Not set" : metricText(value[key], metric)}
            {percent !== null ? ` · ${percent}% of ${key}` : ""}
          </p>
        );
      })}
    </>
  );
}
export function CurrentUsage({ view }: { view?: SampleView }) {
  const s = view?.sample;
  return (
    <section className="evidence-section" aria-label="Current usage">
      <h3>Current usage</h3>
      {!s ? (
        <p className="muted">Metrics unavailable.</p>
      ) : (
        <>
          <p>
            CPU: <MetricsCell view={view} metric="cpu" />
          </p>
          <p>
            Memory: <MetricsCell view={view} metric="memory" />
          </p>
          {s.problem && <p className="muted">{s.problem.message}</p>}
          {view?.last && <p className="warning">Last sample · {view.last}</p>}
          {s.sampledAt && (
            <>
              <p className="muted">
                {s.apiVersion} · Sample {new Date(s.sampledAt).toLocaleString()} · {labels(view!).age}s old · Window {s.windowSeconds}s ·
                Last fetched {new Date(s.fetchedAt).toLocaleString()}
              </p>
              <p className="muted">
                Configured resources captured {s.configuredAt ? new Date(s.configuredAt).toLocaleString() : "Unknown"}; read separately from
                usage.
              </p>
            </>
          )}
          {s.resource.kind === "Node" && (
            <>
              <Comparisons value={s.cpu} metric="cpu" node />
              <Comparisons value={s.memory} metric="memory" node />
            </>
          )}
          {!!s.missingContainers?.length && <p className="warning">Incomplete coverage: {s.missingContainers.join(", ")}</p>}
          {s.podLevelBudget && (
            <p className="muted">Pod-level budgets are configured. Comparisons below use each container’s own request and limit.</p>
          )}
          {s.containers?.map((c) => (
            <div key={c.name}>
              <h4>
                {c.name} · {c.category}
              </h4>
              <p>CPU: {metricText(c.cpu.amount, "cpu")}</p>
              <Comparisons value={c.cpu} metric="cpu" />
              <p>Memory: {metricText(c.memory.amount, "memory")}</p>
              <Comparisons value={c.memory} metric="memory" />
            </div>
          ))}
        </>
      )}
    </section>
  );
}
export function metricColumns<T extends { resource: ResourceRef }>(byIdentity: Record<string, SampleView>) {
  return (["cpu", "memory"] as const).map((metric) => ({
    label: metric === "cpu" ? "CPU" : "Memory",
    value: (row: T) => <MetricsCell view={byIdentity[metricsIdentity(row.resource)]} metric={metric} />,
  }));
}
