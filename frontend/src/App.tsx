/*
Purpose: Compose the real cluster list, Settings and independently owned live tabs.
Used by: The production React entry point.
Responsibilities: Preserve appearance/layout and keep global navigation separate from connection targets.
Important: Startup loads local summaries only; development validation and synthetic fixtures are excluded.
*/
import {useEffect,useRef,useState,type CSSProperties} from 'react';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import {useConnections} from './connections/useConnections';
import {ClusterList} from './connections/ClusterList';
import {ConnectionStatus} from './connections/ConnectionStatus';
import type {CatalogEntry} from './connections/api';
import {SettingsPage} from './settings/SettingsPage';
import {SourcesSettings} from './settings/SourcesSettings';
import {TrustSettings} from './settings/TrustSettings';
import {defaultPreferences,loadPreferences,savePreferences,type Preferences} from './settings/preferences';
import {ClusterTabs} from './workspace/ClusterTabs';
import {Workspace} from './workspace/Workspace';
import {CommandPalette} from './workspace/CommandPalette';
import {commandForKey} from './workspace/commands';
import {ResizeHandle} from './ui/ResizeHandle';
import {ResourceIcon} from './ui/ResourceIcon';
import {InvestigationPanel} from './ai/InvestigationPanel';
import './styles.css';
export default function App(){
 const controller=useConnections();const {state}=controller;
 const [page,setPage]=useState<'clusters'|'workspace'|'settings'>('clusters');
 const [aiOpen,setAiOpen]=useState(false);const aiButton=useRef<HTMLButtonElement>(null);
 const [palette,setPalette]=useState(false);const [trustTarget,setTrustTarget]=useState<CatalogEntry|null>(null);
 const [preferences,setPreferences]=useState(defaultPreferences);const prefsRef=useRef(preferences);prefsRef.current=preferences;const [settingsError,setSettingsError]=useState('');const [saving,setSaving]=useState(false);const [loaded,setLoaded]=useState(false);
 const queue=useRef(Promise.resolve());
 function persist(value:Preferences){setPreferences(value);setSaving(true);queue.current=queue.current.then(()=>savePreferences(value)).then(()=>setSettingsError('')).catch(()=>setSettingsError('Could not save preferences. Check the local configuration file and permissions; your choice is only in memory.')).finally(()=>setSaving(false));}
 function resize(field:'navigationWidth'|'aiWidth'|'dockHeight',value:number){setPreferences(p=>({...p,[field]:value}));}
 function commitSize(){if(loaded&&!settingsError)persist(prefsRef.current);}
 useEffect(()=>{void loadPreferences().then(setPreferences).catch(()=>setSettingsError('Preferences could not be loaded. In-memory defaults are active; the existing file has not been overwritten.')).finally(()=>setLoaded(true));},[]);
 useEffect(()=>{const media=matchMedia('(prefers-color-scheme: dark)');const apply=()=>{document.documentElement.dataset.theme=preferences.appearance==='system'?(media.matches?'dark':'light'):preferences.appearance;};apply();media.addEventListener('change',apply);return ()=>media.removeEventListener('change',apply);},[preferences.appearance]);
 const [startupError,setStartupError]=useState('');
 useEffect(()=>{void Service.Ping().then(()=>Service.FrontendReady()).catch(()=>setStartupError('Desktop bridge unavailable'));},[]);
 useEffect(()=>{const listener=(event:KeyboardEvent)=>{if(commandForKey(event)==='palette'){event.preventDefault();setPalette(true);}};window.addEventListener('keydown',listener);return ()=>window.removeEventListener('keydown',listener);},[]);
 const active=state.active?state.tabs[state.active]:null;
 function choose(entry:CatalogEntry){setPage('workspace');void controller.connect(entry);}
 function close(key:string){void controller.cancel(key);}
 function settings(){setTrustTarget(null);setPage('settings');}
 return <div className="product" style={{'--nav-width':`${preferences.navigationWidth}px`,'--ai-width':`${preferences.aiWidth}px`,'--dock-height':`${preferences.dockHeight}px`} as CSSProperties}>
 <header className="topbar compact-header"><button className="brand" aria-label="KubeSift clusters" onClick={()=>setPage('clusters')}><ResourceIcon kind="Pod" size={23}/> KubeSift</button><ClusterTabs tabs={state.tabs} active={page==='workspace'?state.active:null} onSelect={key=>{controller.commit({type:'focus',key});setPage('workspace');}} onClose={close}/><button onClick={()=>setPage('clusters')}>Choose cluster</button><button aria-pressed={page==='settings'} onClick={()=>page==='settings'?setPage(active?'workspace':'clusters'):settings()}>{page==='settings'?'Back to workspace':'Settings'}</button><button ref={aiButton} className="primary" aria-expanded={aiOpen} aria-controls="ai-investigation-panel" onClick={()=>setAiOpen(!aiOpen)}>{aiOpen?'Hide AI':'Ask AI'}</button></header>
 <div className="identity-bar compact-identity">{page==='workspace'&&active?<><strong title={active.entry.sourceDisplay}>{active.entry.context}</strong><ConnectionStatus view={active.view} unknown={active.pollFailed}/><NamespaceControl key={state.active} namespace={active.namespace} names={active.view?.namespaces??[]} complete={active.view?.namespacesComplete??false} disabled={!active.view?.ref||active.view.state==='connecting'||!!active.view.problem&&active.view.problem.code!=='forbidden'} onSelect={namespace=>void controller.readPods(state.active!,namespace)}/><span className="muted">Workloads / Pods</span></>:<><strong>{page==='settings'?'Settings':'Clusters'}</strong><span className="muted">{Object.keys(state.tabs).length} open contexts</span></>}{startupError&&<span role="alert">{startupError}</span>}</div>
 <div className="main-columns"><div className="page-body">{page==='settings'?<SettingsPage preferences={preferences} error={settingsError} saving={saving||!loaded} onChange={persist} onReset={()=>persist(defaultPreferences())}><SourcesSettings controller={controller}/><TrustSettings key={trustTarget?JSON.stringify([trustTarget.sourceId,trustTarget.context]):'general'} controller={controller} target={trustTarget} onConnect={entry=>{setPage('workspace');const key=JSON.stringify([entry.sourceId,entry.context]);if(state.tabs[key])void controller.retry(key);else void controller.connect(entry);}}/></SettingsPage>:page==='workspace'&&active?<><div className="connection-actions"><span className="source-path">{active.entry.sourceDisplay}</span><span className="spacer"/>{active.view?.state==='connecting'?<button onClick={()=>close(state.active!)}>Cancel connection</button>:<><button onClick={()=>void controller.retry(state.active!)}>Retry</button><button onClick={()=>close(state.active!)}>Disconnect</button></>}{active.view?.problem?.code==='trust_required'&&<button onClick={()=>{setTrustTarget(active.entry);setPage('settings');}}>Review helper in Settings</button>}</div>{(active.error||active.view?.problem||active.pollFailed)&&<div role="alert" className="connection-problem">{active.error||active.view?.problem?.message||(active.pollFailed?'Status unavailable; displayed data may be stale.':'')}</div>}<Workspace key={state.active} tab={active} tabId={state.active!} controller={controller} navigationWidth={preferences.navigationWidth} onResize={n=>resize('navigationWidth',n)} onCommitSize={commitSize}/></>:<ClusterList sources={state.sources} connections={state.tabs} loading={state.loading} error={state.error} onSelect={choose} onSettings={settings}/>}</div>
 <div id="ai-investigation-panel" hidden={!aiOpen} className="ai-host"><ResizeHandle label="AI panel width" reverse value={preferences.aiWidth} min={300} max={560} onChange={n=>resize('aiWidth',n)} onCommit={commitSize}/><InvestigationPanel onClose={()=>{setAiOpen(false);aiButton.current?.focus();}}/></div></div>
 {palette&&<CommandPalette available={false} onCommand={()=>setPage('clusters')} onClose={()=>setPalette(false)}/>}
 </div>;
}
function NamespaceControl({namespace,names,complete,disabled,onSelect}:{namespace:string;names:string[];complete:boolean;disabled:boolean;onSelect:(namespace:string)=>void}){const [manual,setManual]=useState(namespace);useEffect(()=>setManual(namespace),[namespace]);return <form className="namespace-control" onSubmit={e=>{e.preventDefault();if(manual.trim())onSelect(manual.trim());}}><label>Namespace<select aria-label="Discovered namespace" disabled={disabled} value={namespace} onChange={e=>onSelect(e.target.value)}>{!names.includes(namespace)&&<option value={namespace}>{namespace} (manual)</option>}{names.map(name=><option key={name}>{name}</option>)}</select></label><input aria-label="Manual namespace" title={complete?'Enter an explicit namespace':'Namespace discovery incomplete or restricted; enter an explicit namespace'} disabled={disabled} value={manual} onChange={e=>setManual(e.target.value)} placeholder="Namespace"/><button disabled={disabled||!manual.trim()}>Go</button>{!complete&&<span className="muted">Manual entry available</span>}</form>;}
