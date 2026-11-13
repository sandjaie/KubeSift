/*
Purpose: Show NetworkPolicy selectors, peers, ports and rule boundaries.
Used by: NetworkPolicy Overview.
Responsibilities: Distinguish empty rules from absent or unselected directions.
Important: Configuration does not establish enforcement or effective connectivity.
*/
import { selectorText, type Peer, type PolicySpec, type Port } from "./model";

function peerText(peer: Peer) {
  return (
    [
      peer.namespaceSelector && `Namespaces: ${selectorText(peer.namespaceSelector, "All namespaces")}`,
      peer.podSelector && `Pods${peer.namespaceSelector ? "" : " in the policy namespace"}: ${selectorText(peer.podSelector, "All Pods")}`,
      peer.ipBlock &&
        `CIDR: ${peer.ipBlock.cidr ?? "Unknown"}${peer.ipBlock.except?.length ? ` · Except: ${peer.ipBlock.except.join(", ")}` : ""}`,
    ]
      .filter(Boolean)
      .join(" AND ") || "All peers"
  );
}
function portText(port: Port) {
  return `${port.protocol ?? "TCP (default)"} · ${port.port ?? "All ports"}${port.endPort == null ? "" : ` through ${port.endPort}`}`;
}
export function PolicyOverview({ spec }: { spec: PolicySpec }) {
  const ingress = spec.ingress ?? [];
  const egress = spec.egress ?? [];
  const defaulted = !spec.policyTypes?.length;
  const types = defaulted ? ["Ingress", ...(egress.length ? ["Egress"] : [])] : spec.policyTypes!;
  const directions = [
    { name: "Ingress", rules: ingress.map((rule) => ({ peers: rule.from, ports: rule.ports })) },
    { name: "Egress", rules: egress.map((rule) => ({ peers: rule.to, ports: rule.ports })) },
  ];
  return (
    <>
      <h3>NetworkPolicy configuration</h3>
      <p className="network-value">Pod selector: {selectorText(spec.podSelector, "All Pods in the policy namespace")}</p>
      <p>
        Policy types: {types.join(", ")}
        {defaulted && " (API-defaulted interpretation)"}
      </p>
      <p className="muted">Policies combine additively. Enforcement depends on the cluster network implementation.</p>
      {directions.map(({ name, rules }) => (
        <section key={name}>
          <h3>{name}</h3>
          {!types.includes(name) ? (
            <p>Not selected by this policy</p>
          ) : !rules.length ? (
            <p>No allow rules in this policy</p>
          ) : (
            <>
              <p className="muted">
                Rules are alternatives (OR). Peers within a rule are alternatives (OR); selectors in the same peer combine with AND.
              </p>
              <ol>
                {rules.map(({ peers, ports }, index) => (
                  <li key={index} className="network-value">
                    <strong>Rule {index + 1}</strong>
                    {!peers?.length && !ports?.length ? (
                      <p>All peers and ports for this rule</p>
                    ) : (
                      <>
                        {peers?.length ? (
                          <ul aria-label="Alternative peers">
                            {peers.map((peer, i) => (
                              <li key={i}>{peerText(peer)}</li>
                            ))}
                          </ul>
                        ) : (
                          <p>All peers for this rule</p>
                        )}
                        {ports?.length ? (
                          <ul aria-label="Allowed ports">
                            {ports.map((port, i) => (
                              <li key={i}>{portText(port)}</li>
                            ))}
                          </ul>
                        ) : (
                          <p>All ports for this rule</p>
                        )}
                      </>
                    )}
                  </li>
                ))}
              </ol>
            </>
          )}
        </section>
      ))}
    </>
  );
}
