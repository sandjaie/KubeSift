/*
Purpose: Choose one loopback TCP mapping for a Pod or Service.
Used by: The selected Pod or Service's Port forward action.
Responsibilities: Page through Service candidates and present the fixed backend before starting.
Important: No healthy routing, redirect or failover is inferred from selector matches.
*/
import { useEffect, useId, useRef, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import { overviewObject, object, list } from "../workspace/ResourceOverview";
import type { ResourceDetail } from "../workloads/model";
import { readRequest, workloadAPI } from "../workloads/api";
import type { ForwardCandidatePage, ForwardRequest } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

function validPort(text: string, allowZero = false) {
  return /^\d{1,5}$/.test(text) && Number(text) >= (allowZero ? 0 : 1) && Number(text) <= 65535;
}

export function ForwardStart({
  data,
  cluster,
  onClose,
  onStart,
}: {
  data: ResourceDetail;
  cluster: string;
  onClose: () => void;
  onStart: (req: ForwardRequest, signal: AbortSignal) => Promise<unknown>;
}) {
  const resource = data.resource;
  const service = resource.kind === "Service";
  const spec = object(overviewObject(data).spec);
  const ports = list(spec.ports)
    .map(object)
    .filter((port) => (port.protocol ?? "TCP") === "TCP" && typeof port.port === "number");
  const selectorless = service && (spec.type === "ExternalName" || !Object.keys(object(spec.selector)).length);
  const podPorts = [
    ...new Set(
      [...list(spec.containers), ...list(spec.initContainers)]
        .flatMap((c) => list(object(c).ports))
        .map(object)
        .filter((p) => (p.protocol ?? "TCP") === "TCP" && typeof p.containerPort === "number")
        .map((p) => Number(p.containerPort)),
    ),
  ];
  const [remote, setRemote] = useState(!service && podPorts.length === 1 ? String(podPorts[0]) : "");
  const [local, setLocal] = useState("0");
  const [portIndex, setPortIndex] = useState(ports.length === 1 ? "0" : "");
  const [cursor, setCursor] = useState("");
  const [page, setPage] = useState<ForwardCandidatePage>();
  const [chosen, setChosen] = useState("");
  const [error, setError] = useState("");
  const [opening, setOpening] = useState(false);
  const attempt = useRef<AbortController | null>(null);
  const listID = useId();
  const port = portIndex === "" ? undefined : ports[Number(portIndex)];
  const servicePort = typeof port?.port === "number" ? port.port : 0;
  const portName = typeof port?.name === "string" ? port.name : "";
  useEffect(
    () => () => {
      attempt.current?.abort();
    },
    [],
  );
  useEffect(() => {
    if (!service || !servicePort || selectorless) return;
    let current = true;
    const request = readRequest((id) =>
      workloadAPI.ReadForwardCandidates(id, {
        service: resource,
        serviceVersion: data.resourceVersion,
        portName,
        port: servicePort,
        cursor,
      }),
    );
    void request.promise.then(
      (value) => {
        if (!current) return;
        setPage(value);
        if (!cursor && value.complete && value.items?.length === 1) setChosen(value.items[0].pod.uid);
      },
      (error) => {
        if (current) setError(message(error));
      },
    );
    return () => {
      current = false;
      request.cancel();
    };
  }, [service, servicePort, portName, resource, data.resourceVersion, selectorless, cursor]);

  function resetCandidates() {
    setPage(undefined);
    setError("");
    setChosen("");
  }
  const candidate = page?.items?.find((item) => item.pod.uid === chosen);
  const available = !selectorless && validPort(local, true) && (service ? !!candidate : validPort(remote));
  async function open() {
    if (attempt.current || !available) return;
    const req: ForwardRequest = {
      target: { resource: candidate?.pod ?? resource, container: "" },
      localPort: Number(local),
      remotePort: candidate?.remotePort ?? Number(remote),
      service: service ? { resource, resourceVersion: data.resourceVersion, port: servicePort, portName } : null,
    };
    const request = new AbortController();
    attempt.current = request;
    setOpening(true);
    setError("");
    try {
      await onStart(req, request.signal);
      if (!request.signal.aborted) onClose();
    } catch (error) {
      if (!request.signal.aborted) {
        setError(message(error));
        setOpening(false);
      }
    } finally {
      attempt.current = null;
    }
  }

  return (
    <Modal title="Start TCP port forward" onCancel={onClose}>
      <p className="frozen-target">
        {cluster} / {resource.namespace} / {resource.kind} / {resource.name}
      </p>
      {selectorless ? (
        <p>This Service has no selectable Pod. Forward an actual Pod directly instead.</p>
      ) : (
        <form
          className="session-start"
          onSubmit={(event) => {
            event.preventDefault();
            void open();
          }}
        >
          {service ? (
            <>
              {ports.length ? (
                <label>
                  Service TCP port
                  <select
                    value={portIndex}
                    disabled={opening}
                    onChange={(event) => {
                      resetCandidates();
                      setCursor("");
                      setPortIndex(event.target.value);
                    }}
                  >
                    <option value="">Choose port</option>
                    {ports.map((port, index) => (
                      <option key={index} value={index}>
                        {String(port.name || "TCP")} · {String(port.port)}
                      </option>
                    ))}
                  </select>
                </label>
              ) : (
                <p>No TCP Service ports are available. UDP and SCTP are unsupported.</p>
              )}
              {!!servicePort && !page && !error && <p role="status">Reading Pod candidates…</p>}
              {page && (
                <>
                  <p className="muted">Selector matches are candidates. This forward uses one fixed Pod.</p>
                  {page.items?.length ? (
                    <label>
                      Backend Pod
                      <select value={chosen} disabled={opening} onChange={(event) => setChosen(event.target.value)}>
                        <option value="">Choose a Pod</option>
                        {page.items.map(({ pod, remotePort }) => (
                          <option key={pod.uid} value={pod.uid}>
                            {pod.name} · TCP {remotePort}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : (
                    <p>No eligible running Pods on this page.</p>
                  )}
                  {candidate && (
                    <p>
                      Backend: {candidate.pod.namespace}/{candidate.pod.name} · TCP {candidate.remotePort}
                    </p>
                  )}
                  {!page.complete && (
                    <button
                      type="button"
                      disabled={opening}
                      onClick={() => {
                        resetCandidates();
                        setCursor(page.page.nextCursor);
                      }}
                    >
                      Next Pod page
                    </button>
                  )}
                </>
              )}
            </>
          ) : (
            <label>
              Remote TCP port
              <input
                inputMode="numeric"
                value={remote}
                list={listID}
                disabled={opening}
                onChange={(event) => setRemote(event.target.value)}
              />
              <datalist id={listID}>
                {podPorts.map((port) => (
                  <option key={port} value={port} />
                ))}
              </datalist>
            </label>
          )}
          <label>
            Local TCP port (0 = automatic)
            <input inputMode="numeric" value={local} disabled={opening} onChange={(event) => setLocal(event.target.value)} />
          </label>
          <p className="muted">Listens on 127.0.0.1. The allocated address appears in Sessions when ready.</p>
          <button className="primary" type="submit" disabled={!available || opening}>
            {opening ? "Starting…" : "Start forward"}
          </button>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </Modal>
  );
}
