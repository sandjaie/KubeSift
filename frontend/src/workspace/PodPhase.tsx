/*
Purpose: Make actual Pod phases easy to scan without implying readiness.
Used by: The Pod table and selected Pod details.
Responsibilities: Pair a decorative semantic dot with the original phase text.
Important: Unknown values stay visible and neutral; readiness is shown separately.
*/
export function PodPhase({ phase }: { phase: string }) {
  const tone =
    ({ Running: "running", Succeeded: "good", Pending: "warning", Failed: "danger" } as Record<string, string>)[phase] ?? "neutral";
  return (
    <span className="pod-phase">
      <span aria-hidden="true" className={`phase-dot ${tone}`} />
      {phase || "Unknown"}
    </span>
  );
}
