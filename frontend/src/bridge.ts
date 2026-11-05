/*
Purpose: Translate session events and controls across Wails.
Used by: App, session state and terminal controls.
Responsibilities: Describe the event contract and wrap activation, stop and acknowledgement calls.
Important: Subscribe before activation; unsubscribe does not replace an explicit backend stop.
*/
import {Events} from '@wailsio/runtime';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
export interface ConnectionRef {id:string;generation:number}
export interface SessionRef {id:string;connection:ConnectionRef}
export interface SessionEvent {session:SessionRef;sequence:number;kind:string;data:string|null}
export function eventBytes(event:SessionEvent):Uint8Array{return Uint8Array.from(atob(event.data??''),c=>c.charCodeAt(0));}
export function onSessionEvent(receive:(event:SessionEvent)=>void):()=>void{return Events.On('session',event=>receive(event.data as SessionEvent));}
export async function activateSession(ref:SessionRef):Promise<void>{await Service.ActivateSession(ref);}
export async function stopSession(ref:SessionRef):Promise<void>{await Service.StopSession(ref);}
export async function acknowledge(ref:SessionRef,sequence:number):Promise<void>{await Service.AcknowledgeSession(ref,sequence);}
