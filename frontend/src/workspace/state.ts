/*
Purpose: Own in-memory product navigation and preview operations.
Used by: App and workspace interaction controls.
Responsibilities: Preserve per-tab state and reject stale, foreign target identities.
Important: This reducer has no live services or persistence; reviews use copied full keys.
*/
import {inScope,sameKey,type UiState,type TabView,type ResourceKey,type ResourceView,type WorkspaceMode} from './model';
export type UiAction=
 |{type:'preview';tabs:TabView[];resources:ResourceView[]}|{type:'mode';mode:WorkspaceMode}|{type:'page';page:UiState['page']}
 |{type:'selectTab';id:string}|{type:'tab';id:string;patch:Partial<Omit<TabView,'id'>>}|{type:'closeTab';id:string}
 |{type:'review';action:'restart'|'delete';target:ResourceKey}|{type:'confirmReview';id:string}|{type:'dismissReview'}
 |{type:'startSession';kind:'logs'|'terminal'|'forward';target:ResourceKey}|{type:'stopSession';id:string}|{type:'dock';open:boolean}|{type:'cancelClose'}|{type:'confirmClose';id:string};
export function initialUiState():UiState{return {mode:'welcome',page:'resources',activeTab:null,tabs:[],resources:[],sessions:[],review:null,dockOpen:true,aiOpen:false,investigation:null,notice:'',pendingClose:null};}
export function reduceUi(state:UiState,action:UiAction):UiState{
 switch(action.type){
 case 'preview':return state.tabs.length?{...state,mode:'preview',page:'resources'}:{...state,...action,mode:'preview',page:'resources',activeTab:action.tabs[0]?.id??null};
 case 'mode':return {...state,mode:action.mode,page:'resources'};
 case 'page':return {...state,page:action.page};
 case 'selectTab':return state.tabs.some(t=>t.id===action.id)?{...state,activeTab:action.id}:state;
 case 'tab':return {...state,tabs:state.tabs.map(tab=>{if(tab.id!==action.id)return tab;const next={...tab,...action.patch};if(next.selected&&(!state.resources.some(r=>sameKey(r.key,next.selected))||next.selected.clusterId!==tab.id||!inScope(next.selected,next.scope)||next.selected.kind!==next.resourceKind))next.selected=null;return next;})};
 case 'closeTab':if(state.sessions.some(s=>s.target.clusterId===action.id&&s.status==='running'))return {...state,pendingClose:action.id};return removeTab(state,action.id);
 case 'cancelClose':return {...state,pendingClose:null};
 case 'confirmClose':return state.pendingClose===action.id?removeTab(state,action.id):state;
 case 'dock':return {...state,dockOpen:action.open};
 case 'startSession':return state.resources.some(r=>sameKey(r.key,action.target)&&r.key.kind==='Pod')?{...state,dockOpen:true,sessions:[...state.sessions,{id:crypto.randomUUID(),kind:action.kind,target:{...action.target},status:'running',text:action.kind==='terminal'?'$ echo preview\npreview\nInert terminal example — no shell is running.':action.kind==='forward'?'Example 127.0.0.1:8080 → Pod:8080. No listener opened.':'Synthetic captured logs; no stream is running.'}]}:state;
 case 'stopSession':return {...state,sessions:state.sessions.map(s=>s.id===action.id?{...s,status:'stopped'}:s)};
 case 'dismissReview':return {...state,review:null};
 case 'review':return state.resources.some(r=>sameKey(r.key,action.target))?{...state,review:{id:crypto.randomUUID(),action:action.action,target:{...action.target},description:`Simulate ${action.action}. No Kubernetes request will be made.`}}:state;
 case 'confirmReview':{
 const review=state.review;if(!review||review.id!==action.id)return state;
 if(!state.resources.some(r=>sameKey(r.key,review.target)))return {...state,review:null,notice:'Target changed. Select the resource and prepare a fresh review.'};
 const resources=review.action==='delete'?state.resources.filter(r=>!sameKey(r.key,review.target)):state.resources;
 return {...state,resources,review:null,notice:`Simulated ${review.action} for ${review.target.name}. Nothing sent.`,tabs:state.tabs.map(t=>({...t,selected:review.action==='delete'&&sameKey(t.selected,review.target)?null:t.selected}))};
 }
 }
}

function removeTab(state:UiState,id:string):UiState{return {...state,pendingClose:null,tabs:state.tabs.filter(t=>t.id!==id),resources:state.resources.filter(r=>r.key.clusterId!==id),sessions:state.sessions.filter(s=>s.target.clusterId!==id),activeTab:state.activeTab===id?state.tabs.find(t=>t.id!==id)?.id??null:state.activeTab,review:state.review?.target.clusterId===id?null:state.review};}
