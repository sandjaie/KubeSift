/*
Purpose: Observe the original identity associated with a retained deletion result.
Used by: The desktop Check target action.
Responsibilities: Resolve ownership of the route natively and distinguish object absence from read failure.
Important: An observation never mutates or re-executes the operation.
*/
package operation

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *DeleteService) CheckDeleteTarget(ctx context.Context, operationID string) (DeleteObservation, error) {
	result, err := s.coordinator.GetOperationResult(operationID)
	if err != nil || result.Action != "delete" || result.Delete == nil || result.FinishedAt == nil {
		return DeleteObservation{}, model.Fail("not_found", "Completed deletion result is unavailable")
	}
	ctx, release, err := s.coordinator.prepare(ctx)
	if err != nil {
		return DeleteObservation{}, err
	}
	defer release()

	ref := result.Resource
	client, err := newOperationClient(s.connections, ref.Connection)
	if err != nil {
		return DeleteObservation{}, err
	}
	path, plural := deleteTarget(ref)
	var object scalePayload
	var missing operationStatus
	_, err = client.send(ctx, http.MethodGet, path, false, nil, "application/json", &object, &missing)
	obs := DeleteObservation{OperationID: operationID, Resource: ref, CheckedAt: s.now().UTC()}
	group, _, _ := strings.Cut(ref.APIVersion, "/")
	if ref.APIVersion == "v1" {
		group = ""
	}
	switch {
	case errors.Is(err, operationHTTPStatus(http.StatusNotFound)) && objectNotFound(missing, ref.Name, group, plural):
		obs.State = "absent"
	case ref.Kind == "Project" && errors.Is(err, operationHTTPStatus(403)):
		obs.State = "forbidden"
	case err != nil || object.Kind != ref.Kind || object.APIVersion != ref.APIVersion ||
		object.Metadata.Name != ref.Name || object.Metadata.Namespace != ref.Namespace ||
		object.Metadata.UID == "" || len(object.Metadata.UID) > 128 || object.Metadata.Version == "" || len(object.Metadata.Version) > 128:
		obs.State = "unavailable"
		obs.Problem = &model.Problem{Code: "unavailable", Message: "Could not verify the workload on its original connection"}
	case object.Metadata.UID != ref.UID:
		obs.State = "replaced"
	case object.Metadata.Deletion != nil:
		obs.State = "terminating"
	default:
		obs.State = "present"
	}
	return obs, nil
}

func objectNotFound(status operationStatus, name, group, plural string) bool {
	if status.APIVersion != "v1" || status.Kind != "Status" || status.Status != "Failure" ||
		status.Reason != "NotFound" || status.Code != http.StatusNotFound || status.Details == nil {
		return false
	}
	d := status.Details
	groupMatches := d.Group == nil && group == "" || d.Group != nil && *d.Group == group
	return groupMatches && d.Name != nil && *d.Name == name && d.Kind != nil && *d.Kind == plural
}
