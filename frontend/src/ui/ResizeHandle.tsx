/*
Purpose: Resize panes by pointer or keyboard.
Used by: Product navigation, AI and Sessions dock.
Responsibilities: Clamp preferred sizes and label accessible separators.
Important: Effective layout clamping does not overwrite stored wide-screen preferences.
*/
import { useRef } from "react";
export function ResizeHandle({
  label,
  value,
  min,
  max,
  axis = "x",
  reverse = false,
  onChange,
  onCommit,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  axis?: "x" | "y";
  reverse?: boolean;
  onChange: (n: number) => void;
  onCommit: () => void;
}) {
  const drag = useRef<{ at: number; value: number } | null>(null);
  const clamp = (n: number) => Math.round(Math.min(max, Math.max(min, n)));
  return (
    <div
      className={`resize-handle ${axis}`}
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation={axis === "x" ? "vertical" : "horizontal"}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={value}
      onPointerDown={(e) => {
        drag.current = { at: axis === "x" ? e.clientX : e.clientY, value };
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={(e) => {
        if (drag.current)
          onChange(clamp(drag.current.value + ((axis === "x" ? e.clientX : e.clientY) - drag.current.at) * (reverse ? -1 : 1)));
      }}
      onPointerUp={() => {
        drag.current = null;
        onCommit();
      }}
      onPointerCancel={() => {
        drag.current = null;
        onCommit();
      }}
      onKeyDown={(e) => {
        const delta = e.key === "ArrowLeft" || e.key === "ArrowUp" ? -10 : e.key === "ArrowRight" || e.key === "ArrowDown" ? 10 : 0;
        if (delta) {
          e.preventDefault();
          onChange(clamp(value + delta * (reverse ? -1 : 1)));
        }
      }}
      onKeyUp={(e) => {
        if (e.key.startsWith("Arrow")) onCommit();
      }}
    />
  );
}
