/*
Purpose: Define and render the resource navigation registry.
Used by: Workspace.
Responsibilities: Group resource destinations and identify preview availability.
Important: Unimplemented kinds never display invented live data.
*/
export const destinations=[{group:'Workloads',items:[['Pod','Pods',true],['Deployment','Deployments',false],['StatefulSet','StatefulSets',false],['Job','Jobs',false]]},{group:'Networking',items:[['Service','Services',false],['Ingress','Ingresses',false]]},{group:'Configuration',items:[['ConfigMap','ConfigMaps',false],['Secret','Secrets',false]]},{group:'Storage',items:[['PersistentVolumeClaim','Volume claims',false]]},{group:'Cluster & access',items:[['Node','Nodes',false],['Role','Roles & bindings',false]]},{group:'Extensions',items:[['Widget','Custom resources',true],['Helm','Helm',false],['OpenShift','OpenShift',false]]}] as const;
export function ResourceNavigation({kind,onSelect}:{kind:string;onSelect:(kind:string,available:boolean)=>void}){return <nav className="resource-nav" aria-label="Resources">{destinations.map(g=><section key={g.group}><h3>{g.group}</h3>{g.items.map(([key,label,available])=><button key={key} aria-pressed={kind===key} onClick={()=>onSelect(key,available)}><span className="nav-icon">{key==='Pod'?'⬡':'◇'}</span>{label}{!available&&<span className="future" title="Not available in this preview">·</span>}</button>)}</section>)}</nav>;}
