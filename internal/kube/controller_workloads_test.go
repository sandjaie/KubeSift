/*
Purpose: Verify controller-specific workload observations.
Used by: The typed workload reader regression suite.
Responsibilities: Cover revision counts, retries and schedule configuration.
Important: Synthetic status only.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
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
