/*
Purpose: Supply a consistent local SVG icon vocabulary for resource navigation.
Used by: Workspace navigation, Pod rows and the product header.
Responsibilities: Pair distinct silhouettes with readable text labels.
Important: Icons are decorative; accessible names live on their controls.
*/
const paths: Record<string, string> = {
  Pod: "M12 2 3 7v10l9 5 9-5V7z M3 7l9 5 9-5 M12 12v10",
  Deployment: "M4 4h7v7H4z M13 13h7v7h-7z M15 4h5v5 M4 15v5h5",
  ReplicaSet: "M3 7h7v7H3z M14 7h7v7h-7z M8 17h8v4H8z",
  ReplicationController: "M3 5h7v5H3z M14 5h7v5h-7z M8 14h8v5H8z",
  HorizontalPodAutoscaler: "M3 12h18 M7 8l-4 4 4 4 M17 8l4 4-4 4",
  PodDisruptionBudget: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6z M8 12h8",
  ControllerRevision: "M5 4h14v16H5z M8 8h8 M8 12h8 M8 16h4",
  StatefulSet: "M4 4h16v5H4z M4 11h16v4H4z M4 17h16v4H4z",
  DaemonSet: "M4 4h6v6H4z M14 4h6v6h-6z M4 14h6v6H4z M14 14h6v6h-6z",
  CronJob: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z M12 7v5l3 2",
  Job: "M6 3h12v18H6z M9 11l2 2 4-5",
  Service: "M12 3v6 M4 18v-5h16v5 M12 13v5 M2 18h4v4H2z M10 18h4v4h-4z M18 18h4v4h-4z",
  EndpointSlice: "M3 5h18v14H3z M7 9h2v2H7z M11 10h6 M7 14h2v2H7z M11 15h6",
  IngressClass: "M4 4h16v16H4z M7 12h10 M13 8l4 4-4 4",
  NetworkPolicy: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6z M7 12h10 M12 8v8",
  Ingress: "M3 12h17 M14 5l7 7-7 7 M3 4v16",
  Namespace: "M3 6h7l2 2h9v12H3z",
  ResourceQuota: "M4 18h16 M6 15v-4 M12 15V7 M18 15V4",
  LimitRange: "M4 5v14 M20 5v14 M8 12h8 M10 9l-3 3 3 3 M14 9l3 3-3 3",
  ConfigMap: "M4 3h12l4 4v14H4z M8 9h8 M8 13h8 M8 17h5",
  Secret: "M5 10h14v11H5z M8 10V6a4 4 0 0 1 8 0v4 M12 14v3",
  PersistentVolumeClaim: "M4 5c0-4 16-4 16 0v14c0 4-16 4-16 0z M4 5c0 4 16 4 16 0 M4 12c0 4 16 4 16 0",
  PersistentVolume: "M4 5c0-4 16-4 16 0v14c0 4-16 4-16 0z M4 5c0 4 16 4 16 0 M4 12c0 4 16 4 16 0",
  StorageClass: "M4 4h16v5H4z M4 12h16v5H4z M7 6.5h2 M7 14.5h2",
  VolumeAttachment: "M8 8h8v8H8z M4 12h4 M16 12h4 M12 4v4 M12 16v4",
  CSIDriver: "M5 6h14v12H5z M9 10h6 M9 14h6 M2 12h3 M19 12h3",
  CSINode: "M3 4h18v12H3z M8 21h8 M12 16v5",
  Node: "M3 4h18v12H3z M8 21h8 M12 16v5",
  ServiceAccount: "M12 4a4 4 0 1 0 0 8 4 4 0 1 0 0-8 M4 21v-2c0-5 16-5 16 0v2",
  RoleBinding: "M4 8h6v8H4z M14 8h6v8h-6z M10 12h4",
  ClusterRole: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6z M8 9h8 M8 13h8 M10 17h4",
  ClusterRoleBinding: "M12 3v6 M5 13v-2h14v2 M3 14h4v5H3z M17 14h4v5h-4z",
  Role: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6z M8 11l3 3 5-6",
  Event: "M12 3v10 M12 17h.01 M4 21h16L12 3z",
  PriorityClass: "M5 15l7-7 7 7 M5 9l7-7 7 7 M5 21l7-7 7 7",
  RuntimeClass: "M4 5h16v14H4z M7 9l3 3-3 3 M12 15h5",
  Lease: "M12 3a9 9 0 1 0 0 18 9 9 0 1 0 0-18 M12 7v5l4 2",
  CustomResourceDefinition: "M6 3h9l4 4v14H6z M9 10h7 M9 14h7 M9 18h4",
  APIService: "M3 5h6v6H3z M15 13h6v6h-6z M9 8h9v5 M6 11v5h9",
  APIResources: "M8 3h8v5h5v8h-5v5H8v-5H3V8h5z",
  Helm: "M12 2v20 M2 12h20 M5 5l14 14 M5 19 19 5 M18 12a6 6 0 1 1-12 0 6 6 0 0 1 12 0",
  OpenShift: "M8.2 6.2A8 8 0 0 1 18.6 8.4 M18.6 8.4l1.4-3.6-3.6.2 M15.8 17.8A8 8 0 0 1 5.4 15.6 M5.4 15.6l-1.4 3.6 3.6-.2",
};
export function ResourceIcon({ kind, size = 18 }: { kind: string; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinejoin="round"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d={paths[kind] ?? paths.Pod} />
    </svg>
  );
}
