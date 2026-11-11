/*
Purpose: Present appearance settings and persistence failures.
Used by: Product shell.
Responsibilities: Choose System/Light/Dark and host source/label/trust configuration.
Important: Resetting an invalid preferences file is explicit; no trust store is duplicated.
*/
import type {ReactNode} from 'react';
import type {Preferences} from './preferences';
export function SettingsPage({preferences,error,saving,onChange,onReset,children}:{preferences:Preferences;error:string;saving:boolean;onChange:(value:Preferences)=>void;onReset:()=>void;onValidation?:()=>void;children?:ReactNode}){return <main className="settings-page"><span className="eyebrow">WORKSPACE PREFERENCES</span><h1>Settings</h1><p className="muted">Make this workspace yours. Resource and session targets stay unchanged.</p>{error&&<div role="alert" className="settings-error"><p>{error}</p><button onClick={onReset} disabled={saving}>Reset preferences explicitly</button></div>}<section><h2>Appearance</h2><p className="muted">System follows your operating system, including changes while KubeSift is open.</p><div className="appearance-options">{['system','light','dark'].map(v=><button key={v} aria-pressed={preferences.appearance===v} disabled={saving||!!error} onClick={()=>onChange({...preferences,appearance:v})}><span>{v==='system'?'◐':v==='light'?'☀':'☾'}</span>{v[0].toUpperCase()+v.slice(1)}</button>)}</div><p role="status">{saving?'Saving…':error?'Using in-memory preferences; persistence requires attention.':'Preferences ready'}</p></section>{children}</main>;}
