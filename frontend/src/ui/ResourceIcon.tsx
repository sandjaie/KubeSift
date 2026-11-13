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
  StatefulSet: "M4 4h16v5H4z M4 11h16v4H4z M4 17h16v4H4z",
  Job: "M6 3h12v18H6z M9 11l2 2 4-5",
  Service: "M12 3v6 M4 18v-5h16v5 M12 13v5 M2 18h4v4H2z M10 18h4v4h-4z M18 18h4v4h-4z",
  Ingress: "M3 12h17 M14 5l7 7-7 7 M3 4v16",
  ConfigMap: "M4 3h12l4 4v14H4z M8 9h8 M8 13h8 M8 17h5",
  Secret: "M5 10h14v11H5z M8 10V6a4 4 0 0 1 8 0v4 M12 14v3",
  PersistentVolumeClaim: "M4 5c0-4 16-4 16 0v14c0 4-16 4-16 0z M4 5c0 4 16 4 16 0 M4 12c0 4 16 4 16 0",
  Node: "M3 4h18v12H3z M8 21h8 M12 16v5",
  Role: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6z M8 11l3 3 5-6",
  Widget: "M8 3h8v5h5v8h-5v5H8v-5H3V8h5z",
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
