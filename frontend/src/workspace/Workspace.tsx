/*
Purpose: Compose the synthetic cluster explorer.
Used by: Product App after explicit preview entry.
Responsibilities: Keep identity, scope, navigation and selection visible.
Important: Only preview resources enter this component; it imports no desktop services.
*/
import type {Dispatch} from 'react';
import {inScope,sameKey,type UiState} from './model';
import type {UiAction} from './state';
import {ClusterTabs} from './ClusterTabs';
import {ResourceNavigation} from './ResourceNavigation';
import {ResourceTable} from './ResourceTable';
import {StateMessage} from '../ui/Controls';
export function Workspace({state,dispatch}:{state:UiState;dispatch:Dispatch<UiAction>}){
 const tab=state.tabs.find(t=>t.id===state.activeTab);const resources=state.resources;
 if(!tab)return <StateMessage status="empty" title="No preview tabs" detail="Return home and explore the preview to open synthetic clusters again."/>;
 const patch=(value:Partial<typeof tab>)=>dispatch({type:'tab',id:tab.id,patch:value});
 const rows=resources.filter(r=>r.key.clusterId===tab.id&&r.key.kind===tab.resourceKind&&inScope(r.key,tab.scope)&&r.key.name.toLowerCase().includes(tab.filter.toLowerCase()));
 const selected=resources.find(r=>sameKey(r.key,tab.selected));
 return <div className="workspace"><ClusterTabs tabs={state.tabs} active={tab.id} onSelect={id=>dispatch({type:'selectTab',id})} onClose={id=>dispatch({type:'closeTab',id})}/><div className="identity-bar"><strong>Cluster: {tab.label}</strong><span className="muted">{tab.id}</span><label>Namespace <select value={tab.scope.kind==='namespace'?tab.scope.name:tab.scope.kind} onChange={e=>patch({scope:e.target.value==='all'?{kind:'all'}:e.target.value==='cluster'?{kind:'cluster'}:{kind:'namespace',name:e.target.value}})}><option value="all">All namespaces</option><option value="cluster">Cluster scope</option>{[...new Set(resources.filter(r=>r.key.clusterId===tab.id).map(r=>r.key.namespace).filter(Boolean))].map(n=><option key={n} value={n!}>{n}</option>)}</select></label><label>Label <input aria-label="Environment label" value={tab.environment} maxLength={30} onChange={e=>patch({environment:e.target.value})}/></label></div>
 <div className="explorer"><ResourceNavigation kind={tab.resourceKind} onSelect={(kind,available)=>patch({resourceKind:kind,status:available?'ready':'unsupported',selected:null})}/><div className="resource-area"><section className="list-pane"><div className="pane-heading"><h2>{tab.resourceKind==='Pod'?'Pods':tab.resourceKind}</h2><span className="muted">{rows.length} synthetic</span></div><div className="filter-bar"><input aria-label="Filter resources" placeholder="⌕  Filter resources…" value={tab.filter} onChange={e=>patch({filter:e.target.value})}/><label className="view-state">State <select value={tab.status} onChange={e=>patch({status:e.target.value as typeof tab.status})}>{['ready','loading','empty','forbidden','disconnected','stale','unsupported','missing-dependency'].map(v=><option key={v}>{v}</option>)}</select></label></div><p className="table-hint">↑ ↓ Select a row · Full identity stays attached</p>{tab.status==='ready'?<ResourceTable tab={tab} rows={rows} onSelect={selected=>patch({selected})} onScroll={list=>patch({scroll:{...tab.scroll,list}})}/>:<StateMessage status={tab.status} title={`${tab.status} preview`} detail={{loading:'Waiting for a synthetic response.',empty:'This scope has no resources.',forbidden:'This example represents denied access. No trial request was made.',disconnected:'Connection unavailable. No fixture replaces live data.',stale:'This capture is stale. Review the target again before acting.',unsupported:'This destination is not implemented in Phase 1B.', 'missing-dependency':'This workflow needs a dependency unavailable in this preview.',ready:''}[tab.status]}/>}</section><section className="detail-pane">{selected?<><div className="pane-heading"><h2>{selected.key.name}</h2></div><p className="detail-identity">{selected.key.clusterId} / {selected.key.namespace} / {selected.key.uid}</p><dl>{Object.entries(selected.summary).map(([k,v])=><div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl></>:<StateMessage status="empty" title="Select a resource" detail="Inspect its identity, configuration and captured evidence."/>}</section></div></div></div>;
}
