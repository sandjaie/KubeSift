/*
Purpose: Present the minimal Phase 1A desktop validation controls.
Used by: The React entry point.
Responsibilities: Show explicit connections and helper review while retaining independent tab and session state.
Important: Render cluster and helper metadata as text; UI selection must not retarget backend work.
*/
import {useEffect,useState} from 'react';
import {onSessionEvent} from './bridge';
import {reduceEvent,type WorkspaceState} from './state';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import type {ContextSummary} from '../bindings/github.com/sandjaie/KubeSift/internal/config';
import type {PreviewRecord,Entry} from '../bindings/github.com/sandjaie/KubeSift/internal/trust';
import type {ConnectionRef} from '../bindings/github.com/sandjaie/KubeSift/internal/model';

export default function App(){
 const [workspace,setWorkspace]=useState<WorkspaceState>({activeTab:null,tabs:{}});
 const [status,setStatus]=useState('connecting');
 const [path,setPath]=useState('');
 const [review,setReview]=useState<{preview:PreviewRecord,context:ContextSummary}|null>(null);
 const [trusted,setTrusted]=useState<Entry[]>([]);
 const [contexts,setContexts]=useState<ContextSummary[]>([]);
 const [connections,setConnections]=useState<{label:string,ref:ConnectionRef}[]>([]);
 useEffect(()=>onSessionEvent(event=>setWorkspace(current=>reduceEvent(current,event))),[]);
 useEffect(()=>{Service.Ping().then(setStatus).catch(()=>setStatus('Desktop bridge unavailable'));},[]);
 async function load(){try{setContexts((await Service.LoadContexts(path.trim()?[path.trim()]:[]))??[]);setStatus('Local configuration loaded; choose a connection explicitly');}catch{setStatus('Cannot load kubeconfig');}}
 async function open(context:ContextSummary){try{const ref=await Service.OpenConnection(context.snapshotId,context.name);setConnections(old=>[...old.filter(c=>c.ref.id!==ref.id),{label:context.name+' · '+context.namespace,ref}]);setWorkspace(current=>({...current,activeTab:ref.id,tabs:{...current.tabs,[ref.id]:{connection:ref,sessions:{}}}}));setStatus('Connection opened');}catch{setStatus('Connection unavailable. Credential helpers require review before use.');}}
 async function close(ref:ConnectionRef){try{await Service.CloseConnection(ref);setConnections(old=>old.filter(c=>c.ref.id!==ref.id));setWorkspace(current=>{const tabs={...current.tabs};delete tabs[ref.id];return {...current,tabs,activeTab:current.activeTab===ref.id?(Object.keys(tabs)[0]??null):current.activeTab};});}catch{setStatus('Connection already closed or replaced');}}
 async function reviewHelper(context:ContextSummary){try{const preview=await Service.ReviewHelper(context.snapshotId,context.name);setReview({preview,context});}catch{setStatus('Cannot review helper. Check executable availability and configuration.');}}
 async function approve(){if(!review)return;try{await Service.ApproveHelper(review.preview.id,review.context.snapshotId,review.context.name);setReview(null);setStatus('Helper approved. Open the connection explicitly.');}catch{setStatus('Helper review expired or configuration changed. Review again.');}}
 async function listTrust(){try{setTrusted((await Service.TrustedHelpers())??[]);}catch{setStatus('Cannot read trust records; connection remains blocked.');}}
 async function revoke(fingerprint:string){try{await Service.RevokeHelper(fingerprint);setConnections([]);setWorkspace({activeTab:null,tabs:{}});await listTrust();setStatus('Trust revoked; connections closed.');}catch{setStatus('Could not revoke trust.');}}
 return <main><h1>KubeSift · Phase 1A</h1><p role="status">{status}</p>
 <label>Kubeconfig path (blank uses Kubernetes defaults)<input value={path} onChange={e=>setPath(e.target.value)}/></label><button onClick={()=>void load()}>Load local configuration</button>
 <h2>Available contexts</h2><ul>{contexts.map(c=><li key={c.source+c.name}>{c.name} · {c.namespace} · {c.endpoint}<button onClick={()=>void open(c)}>Open {c.name}</button><button onClick={()=>void reviewHelper(c)}>Review helper for {c.name}</button></li>)}</ul>
 {review&&<section aria-label="Credential helper review"><h2>Credential helper review</h2><p>{review.preview.source} · {review.preview.context}</p><p>Executable: {review.preview.executable}</p><p>Arguments: {(review.preview.args??[]).join(' ')}</p><p>Environment: {(review.preview.env??[]).join(' ')}</p><p>Values are redacted. Approval trusts this executable configuration; the helper can run with your account permissions.</p><button onClick={()=>void approve()}>Approve reviewed helper</button><button onClick={()=>setReview(null)}>Cancel review</button></section>}
 <section><h2>Helper trust</h2><button onClick={()=>void listTrust()}>List approved helpers</button><p>Revoking trust closes connections.</p><ul>{trusted.map(entry=><li key={entry.fingerprint}>{entry.label}<button onClick={()=>void revoke(entry.fingerprint)}>Revoke {entry.label}</button></li>)}</ul></section>
 <h2>Explicit connections</h2><p>Active connection: {workspace.activeTab??'none'}</p><ul>{connections.map(c=><li key={c.ref.id}><button onClick={()=>setWorkspace(current=>({...current,activeTab:c.ref.id}))}>Select {c.label}</button> · generation {c.ref.generation}<button onClick={()=>void close(c.ref)}>Close {c.label}</button></li>)}</ul></main>
}
