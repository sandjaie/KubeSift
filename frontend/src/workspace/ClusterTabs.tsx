/*
Purpose: Render independently owned live context tabs.
Used by: The compact product header.
Responsibilities: Focus or close an explicit source/context identity with overflow scrolling.
Important: Equal context names remain distinct through their source tooltips and keys.
*/
import type {ConnectionTab} from '../connections/model';
export function ClusterTabs({tabs,active,onSelect,onClose}:{tabs:Record<string,ConnectionTab>;active:string|null;onSelect:(id:string)=>void;onClose:(id:string)=>void}){return <nav className="cluster-tabs" aria-label="Open clusters">{Object.entries(tabs).map(([key,t])=><div className={key===active?'active':''} key={key}><button title={t.entry.sourceDisplay} aria-pressed={key===active} onClick={()=>onSelect(key)}>{t.entry.context}{t.entry.environment&&<small> · {t.entry.environment}</small>}</button><button aria-label={`Close ${t.entry.context} from ${t.entry.sourceDisplay}`} onClick={()=>onClose(key)}>×</button></div>)}</nav>;}
