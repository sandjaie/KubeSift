/*
Purpose: Show captured Pod volumes and configured container uses.
Used by: Pod Overview.
Responsibilities: Display direct claim links and inert mount/device configuration.
Important: No target read occurs until the user opens an explicit claim reference.
*/
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { podVolumes } from "./model";
import { EvidencePage } from "./StorageOverview";

export function PodStorage({ data, onOpenReference }: { data: ResourceDetail; onOpenReference: (target: ResourceRef) => void }) {
  const rows = podVolumes(data);
  if (!rows.length) return null;
  return (
    <div className="evidence-section" key={JSON.stringify([data.resource.uid, data.resourceVersion])}>
      <h3>Storage</h3>
      <p className="muted">Volumes, mounts and devices describe Pod configuration.</p>
      <EvidencePage
        items={rows}
        render={({ name, type, target, readOnly, uses }) => (
          <section key={name}>
            <h4>
              {name} · {type}
            </h4>
            {target && (
              <p>
                Claim{" "}
                <button className="network-value" onClick={() => onOpenReference(target)}>
                  {target.name}
                </button>
                {" · "}Read only: {readOnly == null ? "Not specified" : String(readOnly)}
              </p>
            )}
            {uses.length ? (
              <EvidencePage
                items={uses}
                render={(use, index) => (
                  <p className="network-value" key={index}>
                    {use}
                  </p>
                )}
              />
            ) : (
              <p>No mount or device associations reported.</p>
            )}
          </section>
        )}
      />
    </div>
  );
}
