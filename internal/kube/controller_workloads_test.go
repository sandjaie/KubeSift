/*
Purpose: Verify controller-specific workload observations.
Used by: The typed workload reader regression suite.
Responsibilities: Cover revision counts, retries and schedule configuration.
Important: Synthetic status only.
*/
package kube

import (
	"context"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	"net/http"
	"strings"
	"testing"
)

func TestBatchStatus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		conditions []batch.JobConditionType
		want       string
	}{
		{"retry", nil, "Active"},
		{"target", []batch.JobConditionType{batch.JobFailureTarget}, "Failure target reached"},
		{"criteria", []batch.JobConditionType{batch.JobSuccessCriteriaMet}, "Success criteria met"},
		{"conflict", []batch.JobConditionType{batch.JobComplete, batch.JobFailed}, "Conflicting status"},
		{"failed", []batch.JobConditionType{batch.JobFailed, batch.JobSuccessCriteriaMet}, "Failed"},
		{"complete", []batch.JobConditionType{batch.JobComplete}, "Complete"},
		{"suspended", []batch.JobConditionType{batch.JobSuspended}, "Suspended"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := batch.Job{Status: batch.JobStatus{Active: 1, Failed: 2}}
			for _, condition := range tc.conditions {
				job.Status.Conditions = append(job.Status.Conditions, batch.JobCondition{Type: condition, Status: core.ConditionTrue})
			}
			summary := summarizeJob(&job)
			if summary.Status != tc.want || summary.Completions != nil || summary.Failed == nil || *summary.Failed != 2 {
				t.Fatalf("unexpected Job summary: %+v", summary)
			}
		})
	}
	if summary := summarizeJob(&batch.Job{}); summary.Failed != nil || summary.Status != "No terminal condition reported" {
		t.Fatal(summary)
	}
	suspended := true
	cron := cronJobRow(model.ConnectionRef{}, batch.CronJob{Spec: batch.CronJobSpec{Suspend: &suspended}, Status: batch.CronJobStatus{Active: []core.ObjectReference{{Name: "still-active"}}}})
	if cron.CronJob.TimeZone != nil || !*cron.CronJob.Suspended || cron.CronJob.Active != 1 {
		t.Fatal(cron.CronJob)
	}
}

func TestRemainingWorkloadCounters(t *testing.T) {
	zero := int32(0)
	row := statefulSetRow(model.ConnectionRef{}, apps.StatefulSet{Spec: apps.StatefulSetSpec{Replicas: &zero}, Status: apps.StatefulSetStatus{Replicas: 3, CurrentReplicas: 1}})
	if row.Total == nil || *row.Total != 3 || row.Desired == nil || *row.Desired != 0 {
		t.Fatalf("unexpected StatefulSet row %+v", row)
	}
	if row := daemonSetRow(model.ConnectionRef{}, apps.DaemonSet{}); row.Desired != nil {
		t.Fatal("unobserved nodes became zero")
	}
	if row := daemonSetRow(model.ConnectionRef{}, apps.DaemonSet{Status: apps.DaemonSetStatus{ObservedGeneration: 1}}); row.Desired == nil || *row.Desired != 0 {
		t.Fatal("observed zero nodes lost")
	}
}

func TestRemainingWorkloadDirectPods(t *testing.T) {
	for _, kind := range []string{"StatefulSet", "DaemonSet", "Job"} {
		t.Run(kind, func(t *testing.T) {
			version := workloadVersion(kind)
			service, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/sample") {
					fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"name":"sample","namespace":"test","uid":"parent"},"spec":{"selector":{"matchLabels":{"app":"sample"}}}}`, version, kind)
					return
				}
				if r.URL.Path != "/api/v1/namespaces/test/pods" || r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("labelSelector") != "app=sample" {
					t.Errorf("unexpected Pod query %v", r.URL)
				}
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"owned","namespace":"test","uid":"owned","ownerReferences":[{"controller":true,"uid":"parent"}]}},{"metadata":{"name":"not-owned","namespace":"test","uid":"other","ownerReferences":[{"controller":false,"uid":"parent"}]}}]}`)
			})
			parent := ResourceRef{Connection: connection, Kind: kind, APIVersion: version, Name: "sample", Namespace: "test", UID: "parent"}
			page, err := service.ReadRelatedPods(context.Background(), RelatedRequest{Resource: parent})
			if err != nil || len(page.Items) != 1 || page.Items[0].Pod.Resource.Name != "owned" || !page.Complete {
				t.Fatalf("owning controller page: %+v %v", page, err)
			}
		})
	}
}
