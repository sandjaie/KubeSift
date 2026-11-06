/*
Purpose: Present captured resource evidence and simulated action affordances.
Used by: Preview Workspace.
Responsibilities: Render overview, events, logs and read-only YAML as text.
Important: Terminal output is inert; no command, stream or mutation is started here.
*/
import {useLayoutEffect,useRef,useState} from 'react';
import {identity,type ResourceView,type TabView} from './model';
export function ResourceDetail({resource,detail,scroll,onScroll,onDetailChange,onAction,onStartSession}:{resource:ResourceView;detail:TabView['detail'];scroll:number;onScroll:(value:number)=>void;onDetailChange:(v:TabView['detail'])=>void;onAction:(action:'restart'|'delete')=>void;onStartSession:(kind:'logs'|'terminal'|'forward')=>void}){
 const [previous,setPrevious]=useState(false);const [follow,setFollow]=useState(true);const [container,setContainer]=useState(resource.containers[0]??'');const pane=useRef<HTMLDivElement>(null);
 useLayoutEffect(()=>{if(pane.current)pane.current.scrollTop=scroll;},[resource.key.clusterId,resource.key.uid]);
 return <div className="resource-detail" ref={pane} onScroll={e=>onScroll(e.currentTarget.scrollTop)}><div className="pane-heading"><h2>⬡ {resource.key.name}</h2></div><p className="detail-identity">{identity(resource.key)}</p><div className="detail-tabs" role="tablist" aria-label="Resource details">{(['overview','events','logs','yaml','terminal'] as const).map(d=><button role="tab" aria-selected={detail===d} key={d} onClick={()=>onDetailChange(d)}>{d==='yaml'?'YAML':d[0].toUpperCase()+d.slice(1)}</button>)}</div>
 <div className="detail-content">{detail==='overview'&&<><h3>Captured resource state</h3><dl>{Object.entries(resource.summary).map(([k,v])=><div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl></>}
 {detail==='events'&&<><h3>Events · synthetic capture</h3>{resource.events.length?resource.events.map(e=><article className="evidence-card" key={e.id}><small>{e.at}</small><h4>{e.reason}</h4><p>{e.message}</p></article>):<p>No captured events.</p>}</>}
 {detail==='logs'&&<><div className="log-controls"><label>Container <select value={container} onChange={e=>setContainer(e.target.value)}>{resource.containers.map(c=><option key={c}>{c}</option>)}</select></label><label><input type="checkbox" checked={previous} onChange={e=>{setPrevious(e.target.checked);if(e.target.checked)setFollow(false);}}/> Previous</label><label><input type="checkbox" checked={follow&&!previous} disabled={previous} onChange={e=>setFollow(e.target.checked)}/> Follow preview</label></div><p className="muted">Synthetic capture · no live stream{previous?' · previous container; follow disabled':''}</p><div className="log-output">{resource.logs.filter(l=>l.container===container&&l.previous===previous).map(l=><pre className={`log-line ${l.severity}`} key={l.id}><span>{l.at} · {l.severity.toUpperCase()}</span>{'\n'}{l.text}</pre>)}</div></>}
 {detail==='yaml'&&<><p className="muted">Read-only synthetic YAML · text selection available</p><pre tabIndex={0} aria-label="Read-only YAML">{resource.yaml}</pre></>}
 {detail==='terminal'&&<><h3>Terminal preview</h3><pre tabIndex={0}>$ echo preview{'\n'}preview{'\n'}No shell running. This is inert example output.</pre></>}
 <div className="resource-actions">{resource.key.kind==='Pod'&&<><button onClick={()=>onStartSession('logs')}>Open logs preview</button><button onClick={()=>onStartSession('terminal')}>Terminal preview</button><button onClick={()=>onStartSession('forward')}>Forward preview</button><button onClick={e=>{e.currentTarget.focus();onAction('restart');}}>Simulate restart…</button></>}<button onClick={e=>{e.currentTarget.focus();onAction('delete');}}>Simulate delete…</button></div></div></div>;
}
