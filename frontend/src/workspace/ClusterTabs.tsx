/*
Purpose: Render independent synthetic cluster tabs.
Used by: Workspace.
Responsibilities: Select or request closure of an explicit tab.
Important: Environment labels are user labels, not health assertions.
*/
import type {TabView} from './model';
export function ClusterTabs({tabs,active,onSelect,onClose}:{tabs:TabView[];active:string|null;onSelect:(id:string)=>void;onClose:(id:string)=>void}){return <nav className="cluster-tabs" aria-label="Preview clusters">{tabs.map(t=><div className={t.id===active?'active':''} key={t.id}><button aria-pressed={t.id===active} onClick={()=>onSelect(t.id)}>{t.label} <span className="muted">[{t.environment||'Unlabeled'}]</span></button><button aria-label={`Close ${t.label}`} onClick={e=>{e.currentTarget.focus();onClose(t.id);}}>×</button></div>)}</nav>;}
