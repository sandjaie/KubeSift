/*
Purpose: Keep streamed resources and output attached to their owning tabs.
Used by: App and the one representative routing test.
Responsibilities: Reject stale identities, retain bounded history and apply safe Pod projections.
Important: The active tab never selects an event target; counters contain byte totals rather than payload copies.
*/
import {eventBytes,type ConnectionRef,type SessionRef,type SessionEvent,type Target} from './bridge';
import type {PodSummary} from '../bindings/github.com/sandjaie/KubeSift/internal/kube';
export interface SessionView{ref:SessionRef;kind:string;label?:string;target?:Target;history:Uint8Array;sequence:number;status:string;error?:string;truncatedBytes:number;resources?:PodSummary[]}
export interface TabState{connection:ConnectionRef;sessions:Record<string,SessionView>;namespace?:string;pods?:PodSummary[];selectedPod?:string;container?:string}
export interface WorkspaceState{activeTab:string|null;tabs:Record<string,TabState>;receivedLogBytes?:number}
export function reduceEvent(state:WorkspaceState,event:SessionEvent):WorkspaceState{
 for(const [tabId,tab] of Object.entries(state.tabs)){
  const current=tab.sessions[event.session.id];
  if(!current||tab.connection.id!==event.session.connection.id||tab.connection.generation!==event.session.connection.generation||current.ref.connection.id!==event.session.connection.id||current.ref.connection.generation!==event.session.connection.generation||event.sequence<=current.sequence)continue;
  const bytes=eventBytes(event);let history=current.history;let status=current.status;let truncatedBytes=current.truncatedBytes;let resources=current.resources;let error=current.error;let receivedLogBytes=state.receivedLogBytes??0;
  if(event.kind==='data'){
   if(current.kind==='logs')receivedLogBytes+=bytes.length;
   const total=history.length+bytes.length;const size=Math.min(total,1<<20);const combined=new Uint8Array(size);const retained=Math.min(history.length,Math.max(0,size-bytes.length));combined.set(history.subarray(history.length-retained),0);combined.set(bytes.subarray(Math.max(0,bytes.length-size)),retained);history=combined;truncatedBytes+=total-size;
  }else if(event.kind==='state'){status=new TextDecoder().decode(bytes);}
  else if(event.kind==='error'){status='error';error=new TextDecoder().decode(bytes);}
  else if(event.kind==='truncated'){const count=Number(new TextDecoder().decode(bytes));truncatedBytes+=Number.isFinite(count)&&count>0?count:0;}
  else if(event.kind==='snapshot'||event.kind==='update'){
   try{const payload=JSON.parse(new TextDecoder().decode(bytes));if(event.kind==='snapshot'){if(!Array.isArray(payload))throw new Error('invalid snapshot');resources=payload as PodSummary[];}else{const update=payload as {type:string;pod:PodSummary};if(!update.pod?.name)throw new Error('invalid update');resources=(resources??[]).filter(pod=>pod.name!==update.pod.name);if(update.type!=='DELETED')resources=[...resources,update.pod];}if(resources.length>100){resources=resources.slice(0,100);status='display limited to 100 Pods';}}catch{error='Invalid Pod event';}
  }
  const updated={...current,history,status,error,truncatedBytes,resources,sequence:event.sequence};return {...state,receivedLogBytes,tabs:{...state.tabs,[tabId]:{...tab,sessions:{...tab.sessions,[event.session.id]:updated}}}};
 }
 return state;
}
