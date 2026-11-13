/*
Purpose: Project typed controller status into read-only workload rows.
Used by: Inventory, details and related Jobs.
Responsibilities: Preserve kind-specific counter meanings and Job condition precedence.
Important: Missing observation is different from reported zero.
*/
package kube

import (
	"github.com/sandjaie/KubeSift/internal/model"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
)

func statefulSetRow(c model.ConnectionRef, set apps.StatefulSet) WorkloadRow {
	status := set.Status
	row := WorkloadRow{Resource: resourceRef(c, "StatefulSet", "apps/v1", set.ObjectMeta), Desired: set.Spec.Replicas, CreatedAt: created(set.ObjectMeta)}
	if status.ObservedGeneration != 0 || len(status.Conditions) != 0 || status.Replicas != 0 || status.CurrentReplicas != 0 || status.ReadyReplicas != 0 || status.UpdatedReplicas != 0 || status.AvailableReplicas != 0 || (status.CollisionCount != nil && *status.CollisionCount != 0) {
		row.Total, row.Ready, row.Updated, row.Available = &status.Replicas, &status.ReadyReplicas, &status.UpdatedReplicas, &status.AvailableReplicas
	}
	return row
}

func daemonSetRow(c model.ConnectionRef, set apps.DaemonSet) WorkloadRow {
	status := set.Status
	row := WorkloadRow{Resource: resourceRef(c, "DaemonSet", "apps/v1", set.ObjectMeta), CreatedAt: created(set.ObjectMeta)}
	if status.ObservedGeneration != 0 || len(status.Conditions) != 0 || status.DesiredNumberScheduled != 0 || status.CurrentNumberScheduled != 0 || status.UpdatedNumberScheduled != 0 || status.NumberReady != 0 || status.NumberAvailable != 0 || status.NumberUnavailable != 0 || status.NumberMisscheduled != 0 || (status.CollisionCount != nil && *status.CollisionCount != 0) {
		row.Desired, row.Current, row.Ready, row.Updated, row.Available = &status.DesiredNumberScheduled, &status.CurrentNumberScheduled, &status.NumberReady, &status.UpdatedNumberScheduled, &status.NumberAvailable
	}
	return row
}

func summarizeJob(job *batch.Job) *JobSummary {
	status := job.Status
	summary := &JobSummary{Status: "No terminal condition reported", Completions: job.Spec.Completions}
	if status.StartTime != nil || status.CompletionTime != nil || len(status.Conditions) != 0 || status.Active != 0 || status.Succeeded != 0 || status.Failed != 0 || (status.Ready != nil && *status.Ready != 0) || (status.Terminating != nil && *status.Terminating != 0) {
		summary.Active, summary.Succeeded, summary.Failed = &status.Active, &status.Succeeded, &status.Failed
	}
	conditions := map[batch.JobConditionType]bool{}
	for _, condition := range status.Conditions {
		if condition.Status == core.ConditionTrue {
			conditions[condition.Type] = true
		}
	}
	switch {
	case conditions[batch.JobComplete] && conditions[batch.JobFailed]:
		summary.Status = "Conflicting status"
	case conditions[batch.JobFailed]:
		summary.Status = "Failed"
	case conditions[batch.JobComplete]:
		summary.Status = "Complete"
	case conditions[batch.JobFailureTarget]:
		summary.Status = "Failure target reached"
	case conditions[batch.JobSuccessCriteriaMet]:
		summary.Status = "Success criteria met"
	case conditions[batch.JobSuspended] || (job.Spec.Suspend != nil && *job.Spec.Suspend):
		summary.Status = "Suspended"
	case status.Active > 0:
		summary.Status = "Active"
	}
	return summary
}

func jobRow(c model.ConnectionRef, job batch.Job) WorkloadRow {
	return WorkloadRow{Resource: resourceRef(c, "Job", "batch/v1", job.ObjectMeta), Job: summarizeJob(&job), CreatedAt: created(job.ObjectMeta)}
}

func cronJobRow(c model.ConnectionRef, job batch.CronJob) WorkloadRow {
	summary := &CronJobSummary{Schedule: job.Spec.Schedule, TimeZone: job.Spec.TimeZone, Suspended: job.Spec.Suspend, Active: len(job.Status.Active)}
	if job.Status.LastScheduleTime != nil {
		summary.LastScheduled = &job.Status.LastScheduleTime.Time
	}
	return WorkloadRow{Resource: resourceRef(c, "CronJob", "batch/v1", job.ObjectMeta), CronJob: summary, CreatedAt: created(job.ObjectMeta)}
}
