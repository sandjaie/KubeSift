/*
Purpose: Render actual basic Pod summaries with keyboard selection.
Used by: The live workspace.
Responsibilities: Show read fields and return the selected UID within the owning tab.
Important: This component never fabricates diagnostics or routes native requests.
*/
import type {PodSummary} from '../../bindings/github.com/sandjaie/KubeSift/internal/kube/models';
import {ResourceIcon} from '../ui/ResourceIcon';
export function ResourceTable({rows,selected,onSelect}:{rows:PodSummary[];selected:string|null;onSelect:(uid:string)=>void}){return <div className="resource-list"><table aria-label="Pods"><thead><tr><th>Name</th><th>Phase</th><th>Ready</th></tr></thead><tbody>{rows.map((pod,index)=><tr key={pod.uid} aria-selected={selected===pod.uid} tabIndex={selected===pod.uid||(!selected&&index===0)?0:-1} onClick={()=>onSelect(pod.uid)} onKeyDown={e=>{if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();const next=Math.max(0,Math.min(rows.length-1,index+(e.key==='ArrowDown'?1:-1)));onSelect(rows[next].uid);(e.currentTarget.parentElement?.children[next] as HTMLElement)?.focus();}if(e.key==='Enter'||e.key===' '){e.preventDefault();onSelect(pod.uid);}}}><td><strong><ResourceIcon kind="Pod" size={14}/> {pod.name}</strong><small>{pod.namespace}</small></td><td>{pod.phase||'Unknown'}</td><td>{pod.ready?'Ready':'Not ready'}</td></tr>)}</tbody></table></div>;}
