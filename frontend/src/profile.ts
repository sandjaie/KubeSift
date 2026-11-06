/*
Purpose: Exercise the real renderer with the approved eight-session local profile.
Used by: App only after the backend enables the explicit fixture validation mode.
Responsibilities: Reuse UI registration/activation/ack paths, collect numeric samples and stop every owned session.
Important: Only the two backend-verified fixture contexts and fixed ks-validation Pod are used; no hidden domain-only soak substitutes for rendering.
*/
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import type {ContextSummary} from '../bindings/github.com/sandjaie/KubeSift/internal/config';
import {listenerCount,type SessionRef,type Target,type ConnectionRef} from './bridge';
import type {WorkspaceState} from './state';
import {terminalSamples} from './validation';
type Controls={open:(context:ContextSummary)=>Promise<ConnectionRef|undefined>;register:(ref:SessionRef,kind:string,target:Target,label:string)=>Promise<void>;stop:(ref:SessionRef)=>Promise<void>;state:()=>WorkspaceState;status:(value:string)=>void};
const delay=(ms:number)=>new Promise<void>(resolve=>setTimeout(resolve,ms));
export async function runProfile(contexts:ContextSummary[],controls:Controls){
 const started=performance.now();let sessions:SessionRef[]=[];let port=0;let rtts:number[]=[];
 const targets:Target[]=[];
 async function sample(phase:number,cycle=0){const state=controls.state();const views=Object.values(state.tabs).flatMap(tab=>Object.values(tab.sessions));const logs=views.filter(view=>view.kind==='logs');await Service.RecordValidation({phase,cycle,elapsedMs:performance.now()-started,listeners:listenerCount(),receivedLogBytes:state.receivedLogBytes??0,retainedLogBytes:logs.reduce((total,view)=>total+view.history.length,0),trimmedLogBytes:logs.reduce((total,view)=>total+view.truncatedBytes,0),maxHistoryBytes:Math.max(0,...logs.map(view=>view.history.length)),terminalRttMs:rtts,forwardPort:port,sessionErrors:views.filter(view=>view.status==='error').length,hostileContentSeen:logs.some(view=>new TextDecoder().decode(view.history).includes('<script>'))});}
 async function register(ref:SessionRef,kind:string,target:Target){sessions.push(ref);await controls.register(ref,kind,target,`PACKAGED PROFILE · ${target.connection.id} / payments / ks-validation / fixture`);return ref;}
 async function startBatch(){for(const target of targets){await register(await Service.WatchPods({...target,name:'',container:''}),'watch',target);for(let i=0;i<2;i++)await register(await Service.StartLogs(target,false,0),'logs',target);}const terminal=await register(await Service.StartTerminal(targets[0]),'terminal',targets[0]);const forward=await Service.StartForward(targets[0],0,8080);port=forward.localPort;await register(forward.session,'forward',targets[0]);return terminal;}
 async function stopBatch(){const owned=sessions;sessions=[];for(const ref of owned)await controls.stop(ref);}
 try{
  if(contexts.length!==2)throw new Error('Both owned fixture contexts are required');
  for(const context of contexts){const ref=await controls.open(context);if(!ref)throw new Error('Fixture connection did not open');await Service.FixtureMarker(ref);targets.push({connection:ref,namespace:'payments',name:'ks-validation',container:'fixture'});}
  await delay(2000);await sample(0);
  const terminal=await startBatch();await delay(2000);await Service.ResizeTerminal(terminal,100,32);rtts=await terminalSamples(terminal);
  const loadStart=performance.now();controls.status('Packaged profile: ten-minute eight-session load running');
  while(performance.now()-loadStart<600000){await sample(1);await delay(Math.min(5000,Math.max(0,600000-(performance.now()-loadStart))));}
  await sample(1);await stopBatch();await delay(2000);
  for(let cycle=1;cycle<=20;cycle++){controls.status(`Packaged profile: cleanup cycle ${cycle}/20`);await startBatch();await delay(1500);await stopBatch();await delay(1000);await sample(2,cycle);}
  await delay(5000);await sample(3,20);controls.status('Packaged profile complete. Numeric evidence saved in the isolated validation directory.');
 }catch(error){controls.status(error instanceof Error?error.message:'Packaged validation failed');try{await sample(4);}finally{await stopBatch();}}
}
