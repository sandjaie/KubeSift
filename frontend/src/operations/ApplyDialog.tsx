/*
Purpose: Show server-side apply evidence inside the shared YAML editor.
Used by: The apply variant of the reviewed YAML workflow.
Responsibilities: Display ownership conflicts, the server diff and a deliberate takeover control.
Important: The control requests a new dry-run; only the editor can claim its token.
*/
import type { ApplyReview } from "./api";
import type { ApplyObservation } from "./applyState";
import { Modal } from "../ui/ReviewDialog";
import { YAMLDiff } from "./YAMLDiff";

export function ApplyReviewContent({
  review,
  reviewing,
  available,
  onTakeover,
}: {
  review: ApplyReview;
  reviewing: boolean;
  available: boolean;
  onTakeover: (enabled: boolean) => void;
}) {
  const conflicts = review.conflicts ?? [];
  return (
    <>
      {conflicts.length > 0 ? (
        <>
          <p role="status">Kubernetes reported field ownership conflicts.</p>
          <ul>
            {conflicts.map(({ field, manager }, i) => (
              <li key={i}>
                <code>{field}</code> · {manager}
              </li>
            ))}
          </ul>
        </>
      ) : (
        <>
          <p role="status">Server dry-run passed.</p>
          {review.beforeYAML === review.serverYAML ? (
            <p>No value changes. This apply may still change field ownership.</p>
          ) : (
            <YAMLDiff before={review.beforeYAML} after={review.serverYAML} />
          )}
          <details>
            <summary>Desired configuration</summary>
            <pre className="yaml-view" tabIndex={0}>
              {review.proposedYAML}
            </pre>
          </details>
          <details>
            <summary>Field ownership changes</summary>
            {review.ownership?.length ? (
              <ul>
                {review.ownership.map(({ change, field }, i) => (
                  <li key={i}>
                    {change} · <code>{field}</code>
                  </li>
                ))}
              </ul>
            ) : (
              <p>No ownership changes shown.</p>
            )}
            {review.ownershipIncomplete && <p>The server’s field ownership evidence is incomplete for this API version.</p>}
            {review.ownershipTruncated && <p>The ownership summary is truncated.</p>}
          </details>
        </>
      )}
      {(conflicts.length > 0 || review.force) && (
        <div>
          <label>
            <input
              type="checkbox"
              checked={review.force}
              disabled={reviewing || !available}
              onChange={(e) => onTakeover(e.target.checked)}
            />
            Take over conflicting fields
          </label>
          <p className="muted">Takeover changes field ownership. Other controllers may change the values again.</p>
        </div>
      )}
    </>
  );
}

export function ApplyResponseDialog({ observation, onClose }: { observation: ApplyObservation; onClose: () => void }) {
  return (
    <Modal title="Apply configuration · Server response" onCancel={onClose}>
      <p>Kubernetes accepted the apply. Its response differs from the dry-run review.</p>
      {observation.observed ? (
        <YAMLDiff before={observation.before} after={observation.observed} />
      ) : (
        <p>The combined response diff exceeds the review limit. Open the target from Operation results to inspect it.</p>
      )}
    </Modal>
  );
}
