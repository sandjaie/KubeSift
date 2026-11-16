/*
Purpose: Render one fixed container terminal and its byte stream.
Used by: The product Sessions dock and development validation.
Responsibilities: Bound pending input, resize in place and acknowledge parsed output.
Important: Hidden terminals still acknowledge frames, but cannot send user input or steal focus.
*/
import { memo, useLayoutEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { Service } from "../bindings/github.com/sandjaie/KubeSift/internal/desktop";
import { activateSession, acknowledge, encodeBytes, eventBytes, onSessionEvent, type SessionRef } from "./bridge";

export default memo(function TerminalView({
  session,
  label,
  visible = true,
  active = true,
  managed = false,
  onAttach,
  onParsed,
  onLegacyReady,
}: {
  session: SessionRef;
  label: string;
  visible?: boolean;
  active?: boolean;
  managed?: boolean;
  onAttach?: (session: SessionRef) => void;
  onParsed?: (session: SessionRef, bytes: Uint8Array) => void;
  onLegacyReady?: (session: SessionRef, mounted: boolean) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const controls = useRef<{ terminal: XTerm; layout: () => void } | null>(null);
  const presentation = useRef({ visible, active });
  presentation.current = { visible, active };
  const [problem, setProblem] = useState("");
  const id = session.id,
    connectionId = session.connection.id,
    generation = session.connection.generation;
  useLayoutEffect(() => {
    const node = host.current;
    if (!node) return;
    let disposed = false;
    let sequence = 0;
    const ref = { id, connection: { id: connectionId, generation } };
    const terminal = new XTerm({
      scrollback: 1000,
      allowProposedApi: false,
      windowOptions: {},
      linkHandler: { activate: () => {} },
      fontSize: 13,
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(node);
    const blockedLinks = terminal.parser.registerOscHandler(8, () => true);
    const blockedClipboard = terminal.parser.registerOscHandler(52, () => true);
    const report = (message: string) => {
      if (!disposed) setProblem(message);
    };
    let pendingBytes = 0;
    let input = Promise.resolve();
    const enqueue = (bytes: Uint8Array) => {
      if (!presentation.current.visible || !presentation.current.active || !node.contains(document.activeElement)) return;
      if (pendingBytes + bytes.length > 64 << 10) {
        report("Terminal input buffer is full. The latest input was not accepted.");
        return;
      }
      pendingBytes += bytes.length;
      input = input
        .then(async () => {
          if (!disposed && presentation.current.active) await Service.TerminalInput(ref, encodeBytes(bytes));
        })
        .catch(() => report("Terminal input was not accepted. Check session state or input backpressure."))
        .finally(() => {
          pendingBytes -= bytes.length;
        });
    };
    const encoder = new TextEncoder();
    const textInput = terminal.onData((data) => enqueue(encoder.encode(data)));
    const binary = terminal.onBinary((data) => enqueue(Uint8Array.from(data, (value) => value.charCodeAt(0))));
    let sent = "";
    let resizing = false;
    let latest = { cols: 0, rows: 0 };
    function resize() {
      if (resizing || disposed || !presentation.current.visible || !presentation.current.active || !latest.cols || !latest.rows) return;
      const { cols, rows } = latest;
      const key = `${cols}/${rows}`;
      if (key === sent) return;
      resizing = true;
      sent = key;
      void Service.ResizeTerminal(ref, cols, rows)
        .catch(() => {
          if (presentation.current.active) report("Terminal resize was not accepted.");
        })
        .finally(() => {
          resizing = false;
          resize();
        });
    }

    const layout = () => {
      if (!presentation.current.visible || !node.clientWidth || !node.clientHeight) return;
      const style = getComputedStyle(node);
      terminal.options.theme = managed
        ? {
            background: style.getPropertyValue("--panel").trim(),
            foreground: style.getPropertyValue("--text").trim(),
            cursor: style.getPropertyValue("--text").trim(),
          }
        : { background: "#102323", foreground: "#e0eeee" };
      const dimensions = fit.proposeDimensions();
      if (!dimensions) return;
      const cols = Math.max(1, Math.min(500, dimensions.cols));
      const rows = Math.max(1, Math.min(500, dimensions.rows));
      if (cols !== terminal.cols || rows !== terminal.rows) terminal.resize(cols, rows);
      latest = { cols, rows };
      resize();
    };
    controls.current = { terminal, layout };
    const observer = new ResizeObserver(layout);
    observer.observe(node);
    const appearance = new MutationObserver(layout);
    if (managed) appearance.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    const off = onSessionEvent((event) => {
      if (
        disposed ||
        event.session.id !== id ||
        event.session.connection.id !== connectionId ||
        event.session.connection.generation !== generation ||
        event.kind !== "data" ||
        event.sequence <= sequence
      )
        return;
      sequence = event.sequence;
      const bytes = eventBytes(event);
      terminal.write(bytes, () => {
        onParsed?.(ref, bytes);
        void acknowledge(ref, event.sequence).catch(() => {
          if (presentation.current.active) report("Terminal output flow control ended.");
        });
      });
    });
    layout();
    if (managed) onAttach?.(ref);
    else
      void activateSession(ref)
        .then(() => {
          if (disposed) return;
          onLegacyReady?.(ref, true);
          terminal.focus();
        })
        .catch(() => report("Terminal could not start or was cancelled."));
    return () => {
      disposed = true;
      controls.current = null;
      onLegacyReady?.(ref, false);
      off();
      observer.disconnect();
      appearance.disconnect();
      textInput.dispose();
      binary.dispose();
      blockedLinks.dispose();
      blockedClipboard.dispose();
      terminal.dispose();
    };
  }, [id, connectionId, generation, managed, onAttach, onParsed, onLegacyReady]);

  useLayoutEffect(() => {
    if (!controls.current) return;
    const { terminal, layout } = controls.current;
    terminal.options.disableStdin = !visible || !active;
    if (!visible && host.current?.contains(document.activeElement)) terminal.blur();
    layout();
  }, [visible, active]);

  return (
    <div className="terminal">
      {!managed && <p>{label}</p>}
      {problem && <p role="alert">{problem}</p>}
      <div ref={host} className="terminal-surface" aria-label={`Terminal ${label}`} />
    </div>
  );
});
