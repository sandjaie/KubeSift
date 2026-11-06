/*
Purpose: Render selectable resources with keyboard navigation.
Used by: Workspace.
Responsibilities: Show status and persist per-tab scroll offsets.
Important: Selection passes a full key; names alone never identify resources.
*/
import {useLayoutEffect,useRef} from 'react';
import {sameKey,type ResourceView,type TabView,type ResourceKey} from './model';
import {StatusBadge} from '../ui/Controls';
export function ResourceTable({tab,rows,onSelect,onScroll}:{tab:TabView;rows:ResourceView[];onSelect:(key:ResourceKey)=>void;onScroll:(value:number)=>void}){
 const scroller=useRef<HTMLDivElement>(null);useLayoutEffect(()=>{if(scroller.current)scroller.current.scrollTop=tab.scroll.list;},[tab.id]);
 return <div ref={scroller} className="resource-list" onScroll={e=>onScroll(e.currentTarget.scrollTop)}><table aria-label="Synthetic resources"><thead><tr><th>Name</th><th>Status</th><th>Ready</th></tr></thead><tbody>{rows.map((r,index)=><tr key={r.key.uid} aria-selected={sameKey(tab.selected,r.key)} tabIndex={sameKey(tab.selected,r.key)||(!tab.selected&&index===0)?0:-1} onClick={()=>onSelect(r.key)} onKeyDown={e=>{if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();const next=Math.max(0,Math.min(rows.length-1,index+(e.key==='ArrowDown'?1:-1)));onSelect(rows[next].key);(e.currentTarget.parentElement?.children[next] as HTMLElement)?.focus();}if(e.key==='Enter'||e.key===' '){e.preventDefault();onSelect(r.key);}}}><td><strong>{r.key.name}</strong><small>{r.key.namespace??'Cluster scoped'}</small></td><td><StatusBadge label={r.status} tone={r.status==='CrashLoopBackOff'?'danger':'success'}/></td><td>{r.ready}</td></tr>)}</tbody></table>{!rows.length&&<p className="empty-table">No matching synthetic resources.</p>}</div>;
}
