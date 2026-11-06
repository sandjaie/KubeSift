/*
Purpose: Define product presentation states independently of live services.
Used by: Product workspace and shared controls.
Responsibilities: Name distinct loading and failure presentations.
Important: Preview identities never become backend session identities.
*/
export type ViewStatus = 'ready'|'loading'|'empty'|'forbidden'|'disconnected'|'stale'|'unsupported'|'missing-dependency';
export type WorkspaceMode='welcome'|'preview'|'validation';
export type NamespaceScope={kind:'all'}|{kind:'namespace';name:string}|{kind:'cluster'};
export type ResourceKey={clusterId:string;apiVersion:string;kind:string;namespace:string|null;name:string;uid:string};
export type ResourceView={key:ResourceKey;status:string;ready:string;containers:string[];summary:Record<string,string>;yaml:string;events:{id:string;at:string;reason:string;message:string}[];logs:{id:string;at:string;severity:'info'|'error';text:string;container:string;previous:boolean}[]};
export type TabView={id:string;label:string;environment:string;scope:NamespaceScope;resourceKind:string;filter:string;selected:ResourceKey|null;detail:'overview'|'events'|'logs'|'yaml'|'terminal';scroll:{list:number;detail:number};status:ViewStatus};
export type PreviewSession={id:string;kind:'logs'|'terminal'|'forward';target:ResourceKey;container?:string;status:'running'|'stopped';text:string};
export type ActionReview={id:string;action:'restart'|'delete';target:ResourceKey;description:string};
export type InvestigationView={id:string;target:ResourceKey|null;collectedAt:string;evidenceIds:string[]};
export type UiState={mode:WorkspaceMode;page:'resources'|'settings';activeTab:string|null;tabs:TabView[];resources:ResourceView[];sessions:PreviewSession[];review:ActionReview|null;dockOpen:boolean;aiOpen:boolean;investigation:InvestigationView|null;notice:string;pendingClose:string|null};
export function sameKey(a:ResourceKey|null,b:ResourceKey|null):boolean{return !!a&&!!b&&a.clusterId===b.clusterId&&a.apiVersion===b.apiVersion&&a.kind===b.kind&&a.namespace===b.namespace&&a.name===b.name&&a.uid===b.uid;}
export function identity(key:ResourceKey):string{return `${key.clusterId} / ${key.namespace??'cluster scope'} / ${key.kind} ${key.name} / ${key.uid}`;}
export function inScope(key:ResourceKey,scope:NamespaceScope):boolean{return scope.kind==='all'?key.namespace!==null:scope.kind==='cluster'?key.namespace===null:key.namespace===scope.name;}
