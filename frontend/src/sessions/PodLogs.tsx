/*
Purpose: Choose an actual Pod container and open explicit log evidence.
Used by: The Pod Logs detail section.
Responsibilities: Load current container configuration and share the application log store.
Important: Selection resets by Pod UID; previous instances never follow.
*/
import { useEffect, useState } from "react";
import type { ResourceRef } from "../workloads/model";
import { readRequest, workloadAPI } from "../workloads/api";
import type { LogSessionsController } from "./useLogSessions";
import { LogViewer } from "./LogViewer";
export function PodLogs({
  resource,
  tab,
  label,
  controller,
}: {
  resource: ResourceRef;
  tab: string;
  label: string;
  controller: LogSessionsController;
}) {
  const [containers, setContainers] = useState<{ name: string; group: string; state: string }[]>([]),
    [container, setContainer] = useState("");
  const [previous, setPrevious] = useState(false),
    [follow, setFollow] = useState(false),
    [tail, setTail] = useState(200),
    [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const request = readRequest((id) => workloadAPI.ReadResource(id, { resource }));
    void request.promise
      .then((data) => {
        if (!active) return;
        const root = JSON.parse(data.objectJSON);
        const values: [string, string, string][] = [
          ["containers", "Regular", "containerStatuses"],
          ["initContainers", "Init", "initContainerStatuses"],
          ["ephemeralContainers", "Ephemeral", "ephemeralContainerStatuses"],
        ];
        const choices = values.flatMap(([key, group, statusKey]) =>
          (root.spec?.[key] ?? []).map((c: { name: string }) => {
            const status = (root.status?.[statusKey] ?? []).find((s: { name: string }) => s.name === c.name);
            return { name: c.name, group, state: status?.state ? Object.keys(status.state).join(", ") : "Unknown state" };
          }),
        );
        setContainers(choices);
        setContainer(choices.length === 1 ? choices[0].name : "");
      })
      .catch(() => {
        if (active) setError("Container evidence unavailable; refresh or inspect Overview.");
      });
    return () => {
      active = false;
      request.cancel();
    };
  }, [resource.connection.id, resource.connection.generation, resource.namespace, resource.uid, resource.name]);
  const sessions = controller.sessions.filter((s) => s.tab === tab && s.request.resource.uid === resource.uid);
  return (
    <div className="evidence-section pod-logs">
      <div className="log-open-controls">
        <select aria-label="Log container" value={container} onChange={(e) => setContainer(e.target.value)}>
          <option value="">Choose container</option>
          {containers.map((c) => (
            <option key={c.name} value={c.name}>
              {c.group}: {c.name} · {c.state}
            </option>
          ))}
        </select>
        <select
          aria-label="Container instance"
          value={previous ? "previous" : "current"}
          onChange={(e) => {
            setPrevious(e.target.value === "previous");
            if (e.target.value === "previous") setFollow(false);
          }}
        >
          <option value="current">Current</option>
          <option value="previous">Previous</option>
        </select>
        <label className="log-tail">
          Lines <input type="number" min={1} max={1000} value={tail} onChange={(e) => setTail(Number(e.target.value))} />
        </label>
        <label className="log-checkbox">
          <input type="checkbox" checked={follow} disabled={previous} onChange={(e) => setFollow(e.target.checked)} /> Follow
        </label>
        <button
          disabled={!container || tail < 1 || tail > 1000}
          onClick={() => {
            setError("");
            void controller.store
              .open({ resource, container, previous, follow, tailLines: tail }, tab, label)
              .catch((e) => setError(e instanceof Error ? e.message : "Cannot open logs"));
          }}
        >
          Open logs
        </button>
      </div>
      {previous && <p>Previous instance logs may be unavailable.</p>}
      {error && <p role="alert">{error}</p>}
      {sessions.map((s) => (
        <LogViewer embedded key={s.id} session={s} store={controller.store} connection={resource.connection} />
      ))}
    </div>
  );
}
