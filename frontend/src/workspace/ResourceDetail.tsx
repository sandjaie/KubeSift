/*
Purpose: Present only fields returned by the basic live Pod reader.
Used by: Workspace after explicit Pod selection.
Responsibilities: Show context, namespace, UID, phase, readiness and container names.
Important: YAML, logs, events and operational actions remain unavailable.
*/
import type {PodSummary} from '../../bindings/github.com/sandjaie/KubeSift/internal/kube/models';
import {ResourceIcon} from '../ui/ResourceIcon';
export function ResourceDetail({pod,context,source,stale}:{pod:PodSummary;context:string;source:string;stale:boolean}){return <div><div className="pane-heading"><h2><ResourceIcon kind="Pod"/> {pod.name}</h2></div><p className="detail-identity">{context} / {pod.namespace}<br/>{source}<br/>UID: {pod.uid}</p>{stale&&<p className="warning detail-identity">Retained data — connection evidence is stale or unavailable.</p>}<dl>{Object.entries({Phase:pod.phase||'Unknown',Readiness:pod.ready?'Ready':'Not ready',Containers:pod.containers?.join(', ')||'None',Marker:pod.marker||'None'}).map(([name,value])=><div key={name}><dt>{name}</dt><dd>{value}</dd></div>)}</dl><p className="detail-identity">Full details, YAML, events, logs, terminal and forwarding are not available yet.</p></div>;}
