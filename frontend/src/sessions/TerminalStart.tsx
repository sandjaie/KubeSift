/*
Purpose: Choose a running container and its exact executable and arguments.
Used by: The selected Pod's Terminal action.
Responsibilities: Capture the target, validate the command form and launch one explicit session.
Important: Commands are argument vectors inside the container, never laptop shell commands.
*/
import { useEffect, useRef, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import { overviewObject, object, list } from "../workspace/ResourceOverview";
import type { ResourceDetail } from "../workloads/model";
import type { TerminalRequest } from "../../bindings/internal/kube/models";

export function TerminalStart({
  data,
  cluster,
  onClose,
  onStart,
}: {
  data: ResourceDetail;
  cluster: string;
  onClose: () => void;
  onStart: (req: TerminalRequest, signal: AbortSignal) => Promise<unknown>;
}) {
  const status = object(overviewObject(data).status);
  const choices = ["containerStatuses", "initContainerStatuses", "ephemeralContainerStatuses"].flatMap((key) =>
    list(status[key])
      .map(object)
      .filter((entry) => object(entry.state).running != null && typeof entry.name === "string")
      .map((entry) => ({
        name: String(entry.name),
        group: key === "containerStatuses" ? "Regular" : key === "initContainerStatuses" ? "Init" : "Ephemeral",
      })),
  );
  const [container, setContainer] = useState(choices.length === 1 ? choices[0].name : "");
  const [executable, setExecutable] = useState("/bin/sh");
  const [args, setArgs] = useState("");
  const [error, setError] = useState("");
  const [opening, setOpening] = useState(false);
  const request = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      request.current?.abort();
    },
    [],
  );

  async function open() {
    if (request.current || !container) return;
    let tail: unknown;
    try {
      tail = args.trim() ? JSON.parse(args) : [];
    } catch {
      setError('Enter arguments as a JSON string array, for example ["-i"].');
      return;
    }
    if (!Array.isArray(tail) || !tail.every((value) => typeof value === "string")) {
      setError('Enter arguments as a JSON string array, for example ["-i"].');
      return;
    }
    const command = [executable, ...tail] as string[];
    if (
      command.length > 32 ||
      command.some((part) => !part.length || part.includes("\0")) ||
      new TextEncoder().encode(command.join("\0")).length >= 8192
    ) {
      setError("Use non-empty command elements without NUL characters, at most 32 elements and 8 KiB.");
      return;
    }
    const attempt = new AbortController();
    request.current = attempt;
    setOpening(true);
    setError("");
    try {
      await onStart({ target: { resource: data.resource, container }, command }, attempt.signal);
      if (!attempt.signal.aborted) onClose();
    } catch (error) {
      if (!attempt.signal.aborted) {
        setError(message(error));
        setOpening(false);
      }
    } finally {
      request.current = null;
    }
  }
  return (
    <Modal title="Open Pod terminal" onCancel={onClose}>
      <p className="frozen-target">
        {cluster} / {data.resource.namespace} / {data.resource.name}
        {container && ` / ${container}`}
      </p>
      <p className="muted">This command runs inside the selected container and can change it.</p>
      {choices.length ? (
        <form
          className="session-start"
          onSubmit={(event) => {
            event.preventDefault();
            void open();
          }}
        >
          <label>
            Running container
            <select value={container} onChange={(event) => setContainer(event.target.value)} disabled={opening}>
              <option value="">Choose container</option>
              {choices.map(({ name, group }) => (
                <option key={name} value={name}>
                  {name} · {group}
                </option>
              ))}
            </select>
          </label>
          <label>
            Executable{" "}
            <input value={executable} onChange={(event) => setExecutable(event.target.value)} disabled={opening} maxLength={8192} />
          </label>
          <label>
            Arguments (optional JSON array){" "}
            <input
              placeholder='["-i"]'
              value={args}
              onChange={(event) => setArgs(event.target.value)}
              disabled={opening}
              maxLength={8192}
            />
          </label>
          <button type="submit" className="primary" disabled={!container || !executable || opening}>
            {opening ? "Opening…" : "Open terminal"}
          </button>
        </form>
      ) : (
        <p role="status">No running container was captured. Refresh the Pod before trying again.</p>
      )}
      {error && <p role="alert">{error}</p>}
    </Modal>
  );
}
