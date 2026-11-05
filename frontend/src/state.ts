/*
Purpose: Keep session output attached to its owning tab.
Used by: App and the representative routing test.
Responsibilities: Match session and connection generations, ignore stale events and bound retained log bytes.
Important: The active tab is presentation state and must never determine an event target.
*/
import {eventBytes,type ConnectionRef,type SessionRef,type SessionEvent} from './bridge';
export interface SessionView{ref:SessionRef;kind:string;history:Uint8Array;sequence:number;status:string;truncatedBytes:number}
export interface TabState{connection:ConnectionRef;sessions:Record<string,SessionView>}
export interface WorkspaceState{activeTab:string|null;tabs:Record<string,TabState>}
export function reduceEvent(state:WorkspaceState,event:SessionEvent):WorkspaceState{
 for(const [tabId,tab] of Object.entries(state.tabs)){
  const current=tab.sessions[event.session.id];
  if(!current||tab.connection.id!==event.session.connection.id||tab.connection.generation!==event.session.connection.generation||current.ref.connection.id!==event.session.connection.id||current.ref.connection.generation!==event.session.connection.generation||event.sequence<=current.sequence)continue;
  const bytes=eventBytes(event);let history=current.history;let status=current.status;let truncatedBytes=current.truncatedBytes;
  if(event.kind==='data'){
   const total=history.length+bytes.length;const size=Math.min(total,1<<20);const combined=new Uint8Array(size);const retained=Math.min(history.length,Math.max(0,size-bytes.length));combined.set(history.subarray(history.length-retained),0);combined.set(bytes.subarray(Math.max(0,bytes.length-size)),retained);history=combined;truncatedBytes+=total-size;
  }else if(event.kind==='state'||event.kind==='error'){status=new TextDecoder().decode(bytes);}
  else if(event.kind==='truncated'){const count=Number(new TextDecoder().decode(bytes));truncatedBytes+=Number.isFinite(count)&&count>0?count:0;}
  const updated={...current,history,status,truncatedBytes,sequence:event.sequence};return {...state,tabs:{...state.tabs,[tabId]:{...tab,sessions:{...tab.sessions,[event.session.id]:updated}}}};
 }
 return state;
}
