/*
Purpose: Copy an exact captured text value.
Used by: YAML and ConfigMap Data views.
Responsibilities: Provide a compact action with accessible feedback.
Important: Copy is explicit and preserves the original capture bytes.
*/
import { useEffect, useState } from "react";

export function CopyTextButton({ text, label, copiedLabel }: { text: string; label: string; copiedLabel: string }) {
  const [notice, setNotice] = useState<{ copied: boolean }>();
  useEffect(() => {
    if (!notice?.copied) return;
    const timer = window.setTimeout(() => setNotice(undefined), 2000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setNotice({ copied: true });
    } catch {
      setNotice({ copied: false });
    }
  }

  return (
    <div className="copy-text">
      <button
        type="button"
        className="copy-text-button"
        data-copied={notice?.copied || undefined}
        aria-label={label}
        title={notice?.copied ? copiedLabel : label}
        onClick={() => void copy()}
      >
        <svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
          {notice?.copied ? (
            <path d="m3 8 3 3 7-7" strokeLinecap="round" strokeLinejoin="round" />
          ) : (
            <>
              <rect x="5.5" y="5.5" width="8.5" height="8.5" rx="1.5" />
              <path d="M3 10H2V3a1 1 0 0 1 1-1h7v1" strokeLinecap="round" />
            </>
          )}
        </svg>
      </button>
      <span role="status" className={notice?.copied === false ? "muted" : "copy-text-status"}>
        {notice && (notice.copied ? copiedLabel : "Copy unavailable. Select text to copy.")}
      </span>
    </div>
  );
}
