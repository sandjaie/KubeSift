/*
Purpose: Inventory Helm records by namespace and storage driver.
Used by: The Helm release list and retained history.
Responsibilities: Complete bounded scans before sorting exact revisions and keep only record identities.
Important: Incomplete metadata never establishes a latest revision.
*/
package helmreader

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	stored "helm.sh/helm/v4/pkg/release/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type metadataRecord struct {
	Metadata metav1.ObjectMeta
}

type metadataList struct {
	Metadata metav1.ListMeta
	Items    metadataRecords
}

type metadataRecords []metadataRecord

func (rows *metadataRecords) UnmarshalJSON(raw []byte) error {
	var decoded []metadataRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	// Kubernetes can encode an empty PartialObjectMetadataList with items: null.
	if decoded == nil {
		decoded = []metadataRecord{}
	}
	*rows = decoded
	return nil
}

type snapshot struct {
	id      string
	scope   Scope
	fetched time.Time
	history map[string][]ReleaseSummary
	latest  []ReleaseSummary
}

func plural(driver string) string {
	if driver == "secret" {
		return "secrets"
	}
	return "configmaps"
}

func metadataPage(ctx context.Context, client kubernetes.Interface, scope Scope, cursor string) (metadataList, error) {
	selector := "owner=helm"
	if scope.ReleaseName != "" {
		selector += ",name=" + scope.ReleaseName
	}
	request := client.CoreV1().RESTClient().Get().Namespace(scope.Namespace).Resource(plural(scope.Driver)).
		Param("labelSelector", selector).Param("limit", "100").Param("continue", cursor)
	raw, err := request.SetHeader("Accept", "application/json;as=PartialObjectMetadataList;g=meta.k8s.io;v=v1").Do(ctx).Raw()
	if apierrors.IsNotAcceptable(err) || apierrors.IsUnsupportedMediaType(err) {
		// Some APIs only return full objects. The bounded transport still applies;
		// the JSON projection discards every body field before storage.
		raw, err = request.SetHeader("Accept", "application/json").Do(ctx).Raw()
	}
	if err != nil {
		if apierrors.IsForbidden(err) {
			return metadataList{}, model.Fail("forbidden", "Kubernetes denied List for Helm storage in this namespace.")
		}
		return metadataList{}, kube.ClassifyError(ctx, err)
	}
	var out metadataList
	if err := json.Unmarshal(raw, &out); err != nil || out.Items == nil || len(out.Items) > pageSize || len(out.Metadata.Continue) > 4096 {
		return metadataList{}, model.Fail("invalid_response", "Kubernetes returned invalid Helm storage metadata.")
	}
	for i := range out.Items {
		m := &out.Items[i].Metadata
		*m = metav1.ObjectMeta{Name: m.Name, Namespace: m.Namespace, UID: m.UID, ResourceVersion: m.ResourceVersion, Labels: map[string]string{
			"owner": m.Labels["owner"], "name": m.Labels["name"], "version": m.Labels["version"],
		}}
	}
	return out, nil
}

func scan(ctx context.Context, client kubernetes.Interface, scope Scope) ([]metadataRecord, error) {
	items := make([]metadataRecord, 0, pageSize)
	cursor := ""
	seen := make(map[string]bool)
	for i := 0; i < snapshotLimit/pageSize+1; i++ {
		page, err := metadataPage(ctx, client, scope, cursor)
		if err != nil {
			return nil, err
		}
		if len(items)+len(page.Items) > snapshotLimit {
			break
		}
		items = append(items, page.Items...)
		cursor = page.Metadata.Continue
		if cursor == "" {
			return items, nil
		}
		if seen[cursor] {
			return nil, model.Fail("invalid_response", "Kubernetes repeated a Helm storage page.")
		}
		seen[cursor] = true
	}
	return nil, model.Fail("capacity", "Helm storage exceeds the scan limit. Enter an exact release name and refresh.")
}

func corruptRow() *model.Problem {
	return &model.Problem{Code: "invalid_response", Message: "Helm record metadata is malformed or ambiguous."}
}

func (s *snapshot) collect(items []metadataRecord) {
	s.history = make(map[string][]ReleaseSummary)
	s.latest = nil
	var malformed []ReleaseSummary
	for _, item := range items {
		meta := item.Metadata
		scope := s.scope
		scope.ReleaseName = meta.Labels["name"]
		if meta.Labels["owner"] != "helm" || meta.Namespace != s.scope.Namespace || (s.scope.ReleaseName != "" && s.scope.ReleaseName != scope.ReleaseName) {
			continue
		}
		if !validReleaseName(scope.ReleaseName) {
			scope.ReleaseName = ""
			malformed = append(malformed, ReleaseSummary{Record: RecordRef{Scope: scope}, Problem: corruptRow()})
			continue
		}
		ref := RecordRef{Scope: scope, Revision: meta.Labels["version"], RecordName: meta.Name, UID: string(meta.UID), ResourceVersion: meta.ResourceVersion}
		row := ReleaseSummary{Record: ref}
		if !ref.valid() {
			row.Problem = corruptRow()
			row.Record = RecordRef{Scope: scope}
		}
		s.history[scope.ReleaseName] = append(s.history[scope.ReleaseName], row)
	}
	names := make([]string, 0, len(s.history))
	for name := range s.history {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rows := s.history[name]
		slices.SortStableFunc(rows, func(a, b ReleaseSummary) int {
			x, y := a.Record.Revision, b.Record.Revision
			if x == "" && y != "" {
				return -1
			}
			if y == "" && x != "" {
				return 1
			}
			if len(x) != len(y) {
				return len(y) - len(x)
			}
			if x > y {
				return -1
			}
			if x < y {
				return 1
			}
			return 0
		})
		for i := 1; i < len(rows); i++ {
			if rows[i-1].Record.Revision == rows[i].Record.Revision {
				rows[i-1].Problem = corruptRow()
				rows[i].Problem = corruptRow()
			}
		}
		s.latest = append(s.latest, rows[0])
	}
	s.latest = append(s.latest, malformed...)
}

func summarize(ref RecordRef, rel *stored.Release) ReleaseSummary {
	result := ReleaseSummary{Record: ref}
	if rel.Info != nil {
		result.Status = string(rel.Info.Status)
		result.UpdatedAt = optionalTime(rel.Info.LastDeployed)
	}
	if rel.Chart != nil && rel.Chart.Metadata != nil {
		result.ChartName = rel.Chart.Metadata.Name
		result.ChartVersion = rel.Chart.Metadata.Version
		result.AppVersion = rel.Chart.Metadata.AppVersion
	}
	return result
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func summarizePage(ctx context.Context, client kubernetes.Interface, rows []ReleaseSummary, page int, budget *decodeBudget) ([]ReleaseSummary, error) {
	if page < 0 || page > snapshotLimit/pageSize || (page > 0 && page*pageSize >= len(rows)) {
		return nil, model.Fail("invalid", "This Helm page is unavailable. Refresh the list.")
	}
	start := page * pageSize
	items := make([]ReleaseSummary, 0, min(pageSize, len(rows)))
	for _, row := range rows[start:min(start+pageSize, len(rows))] {
		if ctx.Err() != nil {
			return nil, kube.ClassifyError(ctx, ctx.Err())
		}
		if row.Problem == nil {
			data, err := readRelease(ctx, client, row.Record, budget)
			if err != nil {
				row.Problem = kube.ClassifyError(ctx, err)
			} else if rel, ok := data.(*stored.Release); ok {
				row = summarize(row.Record, rel)
				if checkSize(row) != nil || len(row.Status)+len(row.ChartName)+len(row.ChartVersion)+len(row.AppVersion) > 8192 {
					row = ReleaseSummary{Record: row.Record, Problem: &model.Problem{Code: "capacity", Message: "Helm summary is oversized."}}
				}
			}
		}
		items = append(items, row)
	}
	return items, nil
}

func (s *Service) ListReleases(ctx context.Context, req ListRequest) (ReleasePage, error) {
	ctx, client, done, err := s.start(ctx, req.Scope)
	if err != nil {
		return ReleasePage{}, err
	}
	defer done()
	key := scopeKey{req.Scope.Connection, req.Scope.Namespace, req.Scope.Driver}
	s.mu.Lock()
	current := s.snapshots[key]
	if req.SnapshotID == "" {
		if req.Page != 0 {
			s.mu.Unlock()
			return ReleasePage{}, model.Fail("invalid", "Refresh the first Helm page.")
		}
		for k := range s.snapshots {
			if k.connection == key.connection {
				delete(s.snapshots, k)
			}
		}
		if len(s.snapshots) >= 16 {
			s.mu.Unlock()
			return ReleasePage{}, model.Fail("capacity", "Too many Helm snapshots are open.")
		}
		current = &snapshot{id: model.ID(), scope: req.Scope}
		s.snapshots[key] = current
	} else if current == nil || current.id != req.SnapshotID || current.scope != req.Scope || current.fetched.IsZero() {
		s.mu.Unlock()
		return ReleasePage{}, model.Fail("stale_target", "The Helm snapshot changed. Refresh the list.")
	}
	s.mu.Unlock()
	if req.SnapshotID == "" {
		items, err := scan(ctx, client, req.Scope)
		if err != nil {
			s.mu.Lock()
			if s.snapshots[key] == current {
				delete(s.snapshots, key)
			}
			s.mu.Unlock()
			return ReleasePage{}, err
		}
		completed := &snapshot{id: current.id, scope: req.Scope, fetched: time.Now().UTC()}
		completed.collect(items)
		s.mu.Lock()
		if ctx.Err() != nil || s.snapshots[key] != current || s.connections.Validate(req.Scope.Connection) != nil {
			s.mu.Unlock()
			return ReleasePage{}, changedRecord()
		}
		s.snapshots[key] = completed
		s.mu.Unlock()
		current = completed
	}
	items, err := summarizePage(ctx, client, current.latest, req.Page, newBudget())
	if err != nil {
		return ReleasePage{}, err
	}
	if err := s.current(ctx, key, current); err != nil {
		return ReleasePage{}, err
	}
	page := ReleasePage{SnapshotID: current.id, Items: items, Page: req.Page, Total: len(current.latest), FetchedAt: current.fetched}
	return page, checkSize(page)
}

func (s *Service) current(ctx context.Context, key scopeKey, value *snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshots[key] != value || ctx.Err() != nil || s.connections.Validate(key.connection) != nil {
		return changedRecord()
	}
	return nil
}

func (s *Service) ReadHistory(ctx context.Context, req HistoryRequest) (HistoryPage, error) {
	ref := req.Record
	ctx, client, done, err := s.start(ctx, ref.Scope)
	if err != nil {
		return HistoryPage{}, err
	}
	defer done()
	key := scopeKey{ref.Connection, ref.Namespace, ref.Driver}
	s.mu.Lock()
	snap := s.snapshots[key]
	s.mu.Unlock()
	if snap == nil {
		return HistoryPage{}, changedRecord()
	}
	rows := snap.history[ref.ReleaseName]
	if !slices.ContainsFunc(rows, func(row ReleaseSummary) bool { return row.Record == ref && row.Problem == nil }) {
		return HistoryPage{}, changedRecord()
	}
	budget := newBudget()
	if _, err := readRelease(ctx, client, ref, budget); err != nil {
		return HistoryPage{}, err
	}
	items, err := summarizePage(ctx, client, rows, req.Page, budget)
	if err != nil {
		return HistoryPage{}, err
	}
	if err := s.current(ctx, key, snap); err != nil {
		return HistoryPage{}, err
	}
	out := HistoryPage{Items: items, Page: req.Page, Total: len(rows), FetchedAt: snap.fetched}
	return out, checkSize(out)
}
