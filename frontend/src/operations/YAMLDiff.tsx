/*
Purpose: Show a bounded line diff for reviewed YAML.
Used by: The Edit and Apply resource reviews.
Responsibilities: Render plain text through the common diff helper.
Important: Values are visible only in the current review.
*/
import { editDiff } from "./editState";

export function YAMLDiff({ before, after }: { before: string; after: string }) {
  return (
    <pre className="yaml-view edit-diff" tabIndex={0}>
      {editDiff(before, after)}
    </pre>
  );
}
