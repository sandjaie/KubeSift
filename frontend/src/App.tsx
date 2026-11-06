/*
Purpose: Compose the product entry and retained local validation workspace.
Used by: React desktop entry point.
Responsibilities: Choose explicit modes and notify native startup readiness.
Important: Opening the shell never opens a cluster; hidden live sessions remain mounted.
*/
import {lazy,Suspense,useEffect,useState,useReducer} from 'react';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import {StatusBadge} from './ui/Controls';
import './styles.css';
import {initialUiState,reduceUi} from './workspace/state';
import {createPreview} from './preview/fixtures';
import {ReviewDialog,Modal} from './ui/ReviewDialog';
import {SessionsDock} from './sessions/SessionsDock';
import {Workspace} from './workspace/Workspace';
const ValidationApp=lazy(()=>import('./ValidationApp'));
export default function App(){
 const [state,dispatch]=useReducer(reduceUi,undefined,initialUiState);const mode=state.mode;const setMode=(mode:typeof state.mode)=>dispatch({type:'mode',mode});const [liveOpened,setLiveOpened]=useState(false);
 const [ready,setReady]=useState('Starting desktop…');
 function validation(){setLiveOpened(true);setMode('validation');}
 useEffect(()=>{void Service.Ping().then(async()=>{await Service.FrontendReady();setReady('Desktop ready');const setup=await Service.ValidationConfiguration();if(setup.autorun){setLiveOpened(true);setMode('validation');}}).catch(()=>setReady('Desktop bridge unavailable'));},[]);
 return <div className="product"><header className="topbar"><button className="brand" onClick={()=>setMode('welcome')}><span>⬡</span> KubeSift</button><span className="muted">Cluster workspace</span><span className="spacer"/><StatusBadge label={ready}/></header>
 <div className="page-body">{mode==='preview'&&<><div className="preview-banner">UI preview · synthetic data · No Kubernetes requests</div><Workspace state={state} dispatch={dispatch}/><SessionsDock sessions={state.sessions} open={state.dockOpen} onToggle={()=>dispatch({type:"dock",open:!state.dockOpen})} onStop={id=>dispatch({type:"stopSession",id})}/></>}<main hidden={mode!=='welcome'} className="welcome"><span className="eyebrow">YOUR CLUSTERS. A CLEARER VIEW.</span><h1>A workspace for<br/>your next investigation.</h1><p>Explore resources, follow evidence and keep every action<br/>bound to the right cluster.</p><div className="welcome-actions"><button className="primary" onClick={()=>dispatch({type:'preview',...createPreview()})}>Explore UI preview →</button><button onClick={validation}>Local validation workspace</button></div><p className="muted">{mode==='preview'?'UI preview · synthetic data. Explorer foundation coming next.':'No cluster connected. Choose a workspace to begin.'}</p></main>
 <div className="validation-workspace" hidden={mode!=='validation'}>{liveOpened&&<Suspense fallback={<p>Loading local validation…</p>}><ValidationApp/></Suspense>}</div></div>
 {state.review&&<ReviewDialog review={state.review} onCancel={()=>dispatch({type:"dismissReview"})} onConfirm={()=>dispatch({type:"confirmReview",id:state.review!.id})}/>}
 {state.pendingClose&&<Modal title="Close preview tab and its sessions?" onCancel={()=>dispatch({type:"cancelClose"})}><p>Stop all synthetic sessions owned by {state.pendingClose} and remove this tab?</p><button onClick={()=>dispatch({type:"confirmClose",id:state.pendingClose!})}>Close tab and stop its sessions</button></Modal>}
 <footer className="statusbar"><span>KubeSift / UI foundation</span><span role="status">{state.notice||"No implicit connections · Explicit targets"}</span></footer></div>;
}
