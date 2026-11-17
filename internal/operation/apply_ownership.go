/*
Purpose: Summarize server-reported field ownership for the fixed apply manager.
Used by: Apply dry-run review.
Responsibilities: Interpret structured causes and compare the before and after field sets.
Important: Different API versions cannot be compared reliably; show incomplete evidence.
*/
package operation

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)

const applyEvidenceLimit = 8 << 10
const applyEvidenceEntries = 100

func applyConflicts(status operationStatus) []ApplyConflict {
	if status.APIVersion != "v1" || status.Kind != "Status" || status.Status != "Failure" ||
		status.Code != 409 || status.Reason != "Conflict" || status.Details == nil ||
		len(status.Details.Causes) == 0 || len(status.Details.Causes) > applyEvidenceEntries {
		return nil
	}
	conflicts := make([]ApplyConflict, 0, len(status.Details.Causes))
	for _, cause := range status.Details.Causes {
		if cause.Reason != "FieldManagerConflict" || !strings.HasPrefix(cause.Field, ".") || len(cause.Field) > 1024 {
			return nil
		}
		description, ok := strings.CutPrefix(cause.Message, "conflict with ")
		if !ok {
			return nil
		}
		quoted, err := strconv.QuotedPrefix(description)
		if err != nil || len(quoted) > 512 {
			return nil
		}
		manager, err := strconv.Unquote(quoted)
		if err != nil || manager == "" {
			return nil
		}
		conflicts = append(conflicts, ApplyConflict{Manager: displayApplyText(manager), Field: displayApplyText(cause.Field)})
	}
	if raw, err := json.Marshal(conflicts); err != nil || len(raw) > applyEvidenceLimit {
		return nil
	}
	return conflicts
}

func displayApplyText(text string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, text)
}

type applyFields struct {
	self, others *fieldpath.Set
	incomplete   bool
}

func managedApplyFields(raw []byte, version string) applyFields {
	out := applyFields{self: fieldpath.NewSet(), others: fieldpath.NewSet()}
	var object struct {
		Metadata struct {
			ManagedFields []struct {
				Manager, Operation, APIVersion, Subresource, FieldsType string
				FieldsV1                                                json.RawMessage
			}
		}
	}
	if json.Unmarshal(raw, &object) != nil || object.Metadata.ManagedFields == nil {
		out.incomplete = true
		return out
	}
	for _, entry := range object.Metadata.ManagedFields {
		if entry.Subresource != "" {
			continue
		}
		if entry.APIVersion != version || entry.FieldsType != "FieldsV1" || len(entry.FieldsV1) == 0 {
			out.incomplete = true
			continue
		}
		set := fieldpath.NewSet()
		if err := set.FromJSON(bytes.NewReader(entry.FieldsV1)); err != nil {
			out.incomplete = true
			continue
		}
		if entry.Manager == applyManager && entry.Operation == "Apply" {
			out.self = out.self.Union(set)
		} else {
			out.others = out.others.Union(set)
		}
	}
	return out
}

func applyOwnership(before, after []byte, version string) (changes []ApplyOwnershipChange, incomplete, truncated bool) {
	old, next := managedApplyFields(before, version), managedApplyFields(after, version)
	oldShared, nextShared := old.self.Intersection(old.others), next.self.Intersection(next.others)
	if old.incomplete || next.incomplete {
		return nil, true, false
	}
	budget := applyEvidenceLimit - 2
	for _, item := range []struct {
		set    *fieldpath.Set
		change string
	}{
		{next.self.Difference(old.self).Difference(nextShared).Union(oldShared.Intersection(next.self).Difference(nextShared)), "Acquired"},
		{old.self.Difference(next.self), "Released"},
		{nextShared.Difference(oldShared), "Shared"},
	} {
		item.set.Iterate(func(path fieldpath.Path) {
			if truncated {
				return
			}
			field := displayApplyText(path.String())
			if field == ".metadata.uid" || field == ".metadata.resourceVersion" {
				return
			}
			change := ApplyOwnershipChange{Field: field, Change: item.change}
			raw, _ := json.Marshal(change)
			if len(changes) >= applyEvidenceEntries || len(raw)+1 > budget {
				truncated = true
				return
			}
			budget -= len(raw) + 1
			changes = append(changes, change)
		})
	}
	return changes, incomplete, truncated
}
