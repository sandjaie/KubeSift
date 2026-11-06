/*
Purpose: Compose the product entry and retained local validation workspace.
Used by: React desktop entry point.
Responsibilities: Choose explicit modes and notify native startup readiness.
Important: Opening the shell never opens a cluster; hidden live sessions remain mounted.
*/
import {lazy,Suspense,useEffect,useState,useReducer,useRef,type CSSProperties} from 'react';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import {StatusBadge} from './ui/Controls';
import './styles.css';
import {initialUiState,reduceUi} from './workspace/state';
import {createPreview} from './preview/fixtures';
import {ReviewDialog,Modal} from './ui/ReviewDialog';
import {SessionsDock} from './sessions/SessionsDock';
import {SettingsPage} from './settings/SettingsPage';
import {defaultPreferences,loadPreferences,savePreferences,type Preferences} from './settings/preferences';
import {ResizeHandle} from './ui/ResizeHandle';
import {Workspace} from './workspace/Workspace';
const ValidationApp=lazy(()=>import('./ValidationApp'));
export default function App(){
 const [state,dispatch]=useReducer(reduceUi,undefined,initialUiState);const mode=state.mode;const setMode=(mode:typeof state.mode)=>dispatch({type:'mode',mode});const [liveOpened,setLiveOpened]=useState(false);
 const [preferences,setPreferences]=useState(defaultPreferences);const prefsRef=useRef(preferences);prefsRef.current=preferences;const [settingsError,setSettingsError]=useState('');const [saving,setSaving]=useState(false);const [loaded,setLoaded]=useState(false);
 const queue=useRef(Promise.resolve());
 function persist(value:Preferences){setPreferences(value);setSaving(true);queue.current=queue.current.then(()=>savePreferences(value)).then(()=>setSettingsError('')).catch(()=>setSettingsError('Could not save preferences. Check the local configuration file and permissions; your choice is only in memory.')).finally(()=>setSaving(false));}
 function resize(field:'navigationWidth'|'aiWidth'|'dockHeight',value:number){setPreferences(p=>({...p,[field]:value}));}
 function commitSize(){if(loaded&&!settingsError)persist(prefsRef.current);}
 useEffect(()=>{void loadPreferences().then(setPreferences).catch(()=>setSettingsError('Preferences could not be loaded. In-memory defaults are active; the existing file has not been overwritten.')).finally(()=>setLoaded(true));},[]);
 useEffect(()=>{const media=matchMedia('(prefers-color-scheme: dark)');const apply=()=>{document.documentElement.dataset.theme=preferences.appearance==='system'?(media.matches?'dark':'light'):preferences.appearance;};apply();media.addEventListener('change',apply);return ()=>media.removeEventListener('change',apply);},[preferences.appearance]);
 const [ready,setReady]=useState('Starting desktop…');
 function validation(){setLiveOpened(true);setMode('validation');}
 useEffect(()=>{void Service.Ping().then(async()=>{await Service.FrontendReady();setReady('Desktop ready');const setup=await Service.ValidationConfiguration();if(setup.autorun){setLiveOpened(true);setMode('validation');}}).catch(()=>setReady('Desktop bridge unavailable'));},[]);
 return <div className="product" style={{"--nav-width":`${preferences.navigationWidth}px`,"--ai-width":`${preferences.aiWidth}px`,"--dock-height":`${preferences.dockHeight}px`} as CSSProperties}><header className="topbar"><button className="brand" onClick={()=>setMode('welcome')}><span>⬡</span> KubeSift</button><span className="muted">Cluster workspace</span><span className="spacer"/><StatusBadge label={ready}/><button onClick={()=>dispatch({type:"page",page:state.page==="settings"?"resources":"settings"})}>{state.page==="settings"?"Back to workspace":"Settings"}</button></header>
 <div className="page-body">{state.page==="settings"&&<SettingsPage preferences={preferences} error={settingsError} saving={saving||!loaded} onChange={persist} onReset={()=>persist(defaultPreferences())} onValidation={validation}/>}{mode==='preview'&&state.page==='resources'&&<><div className="preview-banner">UI preview · synthetic data · No Kubernetes requests</div><Workspace state={state} dispatch={dispatch} navigationWidth={preferences.navigationWidth} onResize={n=>resize("navigationWidth",n)} onCommitSize={commitSize}/><ResizeHandle label="Sessions dock height" axis="y" reverse value={preferences.dockHeight} min={120} max={480} onChange={n=>resize("dockHeight",n)} onCommit={commitSize}/><SessionsDock sessions={state.sessions} open={state.dockOpen} onToggle={()=>dispatch({type:"dock",open:!state.dockOpen})} onStop={id=>dispatch({type:"stopSession",id})}/></>}<main hidden={mode!=='welcome'||state.page!=='resources'} className="welcome"><span className="eyebrow">YOUR CLUSTERS. A CLEARER VIEW.</span><h1>A workspace for<br/>your next investigation.</h1><p>Explore resources, follow evidence and keep every action<br/>bound to the right cluster.</p><div className="welcome-actions"><button className="primary" onClick={()=>dispatch({type:'preview',...createPreview()})}>Explore UI preview →</button><button onClick={validation}>Local validation workspace</button></div><p className="muted">{mode==='preview'?'UI preview · synthetic data. Explorer foundation coming next.':'No cluster connected. Choose a workspace to begin.'}</p></main>
 <div className="validation-workspace" hidden={mode!=='validation'||state.page!=='resources'}>{liveOpened&&<Suspense fallback={<p>Loading local validation…</p>}><ValidationApp/></Suspense>}</div></div>
 {state.review&&<ReviewDialog review={state.review} onCancel={()=>dispatch({type:"dismissReview"})} onConfirm={()=>dispatch({type:"confirmReview",id:state.review!.id})}/>}
 {state.pendingClose&&<Modal title="Close preview tab and its sessions?" onCancel={()=>dispatch({type:"cancelClose"})}><p>Stop all synthetic sessions owned by {state.pendingClose} and remove this tab?</p><button onClick={()=>dispatch({type:"confirmClose",id:state.pendingClose!})}>Close tab and stop its sessions</button></Modal>}
 <footer className="statusbar"><span>KubeSift / UI foundation</span><span role="status">{state.notice||"No implicit connections · Explicit targets"}</span></footer></div>;
}
