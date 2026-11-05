import {useEffect,useState} from 'react';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import type {ContextSummary} from '../bindings/github.com/sandjaie/KubeSift/internal/config';
import type {ConnectionRef} from '../bindings/github.com/sandjaie/KubeSift/internal/model';

export default function App(){
 const [status,setStatus]=useState('connecting');
 const [path,setPath]=useState('');
 const [contexts,setContexts]=useState<ContextSummary[]>([]);
 const [connections,setConnections]=useState<{label:string,ref:ConnectionRef}[]>([]);
 useEffect(()=>{Service.Ping().then(setStatus).catch(()=>setStatus('Desktop bridge unavailable'));},[]);
 async function load(){try{setContexts((await Service.LoadContexts(path.trim()?[path.trim()]:[]))??[]);setStatus('Local configuration loaded; choose a connection explicitly');}catch{setStatus('Cannot load kubeconfig');}}
 async function open(context:ContextSummary){try{const ref=await Service.OpenConnection(context.name);setConnections(old=>[...old.filter(c=>c.ref.id!==ref.id),{label:context.name+' · '+context.namespace,ref}]);setStatus('Connection opened');}catch{setStatus('Connection unavailable. Credential helpers require review before use.');}}
 async function close(ref:ConnectionRef){try{await Service.CloseConnection(ref);setConnections(old=>old.filter(c=>c.ref.id!==ref.id));}catch{setStatus('Connection already closed or replaced');}}
 return <main><h1>KubeSift · Phase 1A</h1><p role="status">{status}</p>
 <label>Kubeconfig path (blank uses Kubernetes defaults)<input value={path} onChange={e=>setPath(e.target.value)}/></label><button onClick={()=>void load()}>Load local configuration</button>
 <h2>Available contexts</h2><ul>{contexts.map(c=><li key={c.source+c.name}>{c.name} · {c.namespace} · {c.endpoint}<button onClick={()=>void open(c)}>Open {c.name}</button></li>)}</ul>
 <h2>Explicit connections</h2><ul>{connections.map(c=><li key={c.ref.id}>{c.label} · generation {c.ref.generation}<button onClick={()=>void close(c.ref)}>Close {c.label}</button></li>)}</ul></main>
}
