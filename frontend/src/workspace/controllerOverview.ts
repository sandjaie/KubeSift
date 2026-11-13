/*
Purpose: Select observed counters and configuration for remaining workload overviews.
Used by: The compact shared workload details.
Responsibilities: Keep node, Pod, revision and scheduling evidence distinct.
Important: Status omissions never imply successful execution.
*/
import type { ResourceDetail } from "../workloads/model";

type Fields = Record<string, unknown>;
const observation = (status: Fields, counters: string[]) =>
  Number(status.observedGeneration) > 0 ||
  (Array.isArray(status.conditions) && status.conditions.length > 0) ||
  counters.some((key) => typeof status[key] === "number" && status[key] !== 0);

export function controllerOverview(data: ResourceDetail, spec: Fields, status: Fields) {
  const kind = data.resource.kind;
  const configured = (value: unknown) => value ?? "Not configured";
  if (kind === "StatefulSet") {
    const observed = observation(status, [
      "replicas",
      "currentReplicas",
      "readyReplicas",
      "updatedReplicas",
      "availableReplicas",
      "collisionCount",
    ]);
    const count = (key: string) => (observed ? (status[key] ?? 0) : undefined);
    return {
      summary: {
        "Ready Pods": count("readyReplicas"),
        "Desired Pods": spec.replicas,
        "Total Pods": count("replicas"),
        "Updated Pods": count("updatedReplicas"),
        "Available Pods": count("availableReplicas"),
      },
      facts: {
        "Current revision Pods": count("currentReplicas"),
        "Current revision": status.currentRevision,
        "Update revision": status.updateRevision,
      },
      configuration: {
        "Service name": configured(spec.serviceName),
        "Pod management policy": configured(spec.podManagementPolicy),
        Ordinals: configured(spec.ordinals),
        "Update strategy": configured(spec.updateStrategy),
        "Minimum ready seconds": configured(spec.minReadySeconds),
        "Volume claim templates": configured(spec.volumeClaimTemplates),
        "Volume claim retention": configured(spec.persistentVolumeClaimRetentionPolicy),
      },
    };
  }
  if (kind === "DaemonSet") {
    const observed = observation(status, [
      "desiredNumberScheduled",
      "currentNumberScheduled",
      "updatedNumberScheduled",
      "numberReady",
      "numberAvailable",
      "numberUnavailable",
      "numberMisscheduled",
      "collisionCount",
    ]);
    const count = (key: string) => (observed ? (status[key] ?? 0) : undefined);
    return {
      summary: {
        "Ready nodes": count("numberReady"),
        "Desired nodes": count("desiredNumberScheduled"),
        "Scheduled nodes": count("currentNumberScheduled"),
        "Updated nodes": count("updatedNumberScheduled"),
        "Available nodes": count("numberAvailable"),
      },
      facts: { "Unavailable nodes": count("numberUnavailable"), "Misscheduled nodes": count("numberMisscheduled") },
      configuration: { "Update strategy": configured(spec.updateStrategy), "Minimum ready seconds": configured(spec.minReadySeconds) },
    };
  }
  if (kind === "Job") {
    return {
      summary: {
        Status: data.job?.status,
        "Active Pods": data.job?.active,
        Succeeded: data.job?.succeeded,
        "Failed attempts": data.job?.failed,
        Completions: data.job?.completions ?? "Not specified",
      },
      facts: {
        Started: status.startTime,
        Completed: status.completionTime,
        "Completed indexes": status.completedIndexes,
        "Failed indexes": status.failedIndexes,
        "Ready Pods": status.ready,
        "Terminating Pods": status.terminating,
      },
      configuration: {
        Parallelism: configured(spec.parallelism),
        Suspended: configured(spec.suspend),
        "Completion mode": configured(spec.completionMode),
        "Backoff limit": configured(spec.backoffLimit),
        "Backoff limit per index": configured(spec.backoffLimitPerIndex),
        "Max failed indexes": configured(spec.maxFailedIndexes),
        "Active deadline seconds": configured(spec.activeDeadlineSeconds),
        "TTL after finished": configured(spec.ttlSecondsAfterFinished),
        "Pod failure policy": configured(spec.podFailurePolicy),
        "Success policy": configured(spec.successPolicy),
        "Pod replacement policy": configured(spec.podReplacementPolicy),
        "Managed by": configured(spec.managedBy),
      },
    };
  }
  if (kind === "CronJob") {
    return {
      summary: {
        Schedule: spec.schedule,
        "Time zone": spec.timeZone ?? "Controller default (unknown)",
        Suspended: spec.suspend,
        "Active Job references": Array.isArray(status.active) ? status.active.length : 0,
      },
      facts: {
        "Concurrency policy": spec.concurrencyPolicy,
        "Last scheduled": status.lastScheduleTime,
        "Last successful": status.lastSuccessfulTime,
      },
      configuration: {
        "Starting deadline seconds": configured(spec.startingDeadlineSeconds),
        "Successful Jobs history limit": configured(spec.successfulJobsHistoryLimit),
        "Failed Jobs history limit": configured(spec.failedJobsHistoryLimit),
        "Job template": spec.jobTemplate,
      },
    };
  }
  return null;
}
