/*
Purpose: Allocate space between the Pod list and detail pane in either layout.
Used by: Workspace.
Responsibilities: Measure the responsive axis, clamp displayed shares and expose a draggable separator.
Important: Window changes never save over the user's independent width and height preferences.
*/
import { useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { ResizeHandle } from "../ui/ResizeHandle";

export function ResourceSplit({
  widthPercent,
  heightPercent,
  onResize,
  onCommit,
  children,
}: {
  widthPercent: number;
  heightPercent: number;
  onResize: (axis: "x" | "y", value: number) => void;
  onCommit: () => void;
  children: [ReactNode, ReactNode];
}) {
  const container = useRef<HTMLDivElement>(null);
  const [geometry, setGeometry] = useState({ axis: "x" as "x" | "y", extent: 600 });
  useLayoutEffect(() => {
    const element = container.current;
    if (!element) return;
    const measure = () => {
      const axis = getComputedStyle(element).flexDirection === "column" ? "y" : "x";
      const extent = Math.max(1, (axis === "x" ? element.clientWidth : element.clientHeight) - 5);
      setGeometry((old) => (old.axis === axis && old.extent === extent ? old : { axis, extent }));
    };
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    window.addEventListener("resize", measure);
    measure();
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  const { axis, extent } = geometry;
  const detailMinimum = axis === "x" ? 300 : 280;
  const listMinimum = axis === "x" ? 280 : 230;
  const min = Math.min(50, Math.max(20, Math.ceil((detailMinimum / extent) * 100)));
  const max = Math.max(min, Math.min(80, Math.floor(((extent - listMinimum) / extent) * 100)));
  const preferred = axis === "x" ? widthPercent : heightPercent;
  const displayed = Math.min(max, Math.max(min, preferred));
  return (
    <div
      ref={container}
      className="resource-area resource-split"
      style={{ "--detail-size": `${(extent * displayed) / 100}px` } as CSSProperties}
    >
      {children[0]}
      <ResizeHandle
        label="Pod details size"
        axis={axis}
        reverse
        value={displayed}
        min={min}
        max={max}
        pixelsPerUnit={extent / 100}
        step={2}
        onChange={(value) => onResize(axis, value)}
        onCommit={onCommit}
      />
      {children[1]}
    </div>
  );
}
