/*
Purpose: Keep synthetic session ownership visible across navigation.
Used by: Product shell in preview mode.
Responsibilities: Collapse without deletion and stop individual captured sessions.
Important: These inert previews never allocate backend sessions or network listeners.
*/
import { identity, type PreviewSession } from "../workspace/model";
export function SessionsDock({
  sessions,
  open,
  onToggle,
  onStop,
}: {
  sessions: PreviewSession[];
  open: boolean;
  onToggle: () => void;
  onStop: (id: string) => void;
}) {
  return (
    <section className="sessions-dock">
      <header>
        <button onClick={onToggle} aria-expanded={open}>
          {open ? "▾" : "▸"} Sessions <span className="muted">{sessions.length} · synthetic</span>
        </button>
        <span className="muted">Targets stay fixed across tabs</span>
      </header>
      <div hidden={!open} className="session-cards">
        {!sessions.length && <p className="muted">Open a logs, terminal or forward preview from a selected Pod.</p>}
        {sessions.map((s) => (
          <article key={s.id}>
            <header>
              <strong>{s.kind} preview</strong>
              <span>{s.status}</span>
              <button
                disabled={s.status === "stopped"}
                onClick={() => onStop(s.id)}
                aria-label={`Stop ${s.kind} ${s.target.clusterId} ${s.target.name}`}
              >
                Stop
              </button>
            </header>
            <p className="detail-identity">{identity(s.target)}</p>
            <pre>{s.text}</pre>
          </article>
        ))}
      </div>
    </section>
  );
}
