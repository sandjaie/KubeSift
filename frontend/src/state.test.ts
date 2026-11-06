/*
Purpose: Prove that tab switches and delayed events cannot mix session output.
Used by: The single frontend routing test in Vitest.
Responsibilities: Verify explicit Pod/session targeting, cumulative errors, owned cleanup and parsed terminal timing.
Important: Keep this a behavior test; do not expand it into an exhaustive UI or snapshot suite.
*/
import {it,expect,vi} from 'vitest';
import {CancellablePromise} from '@wailsio/runtime';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import {terminalReady,terminalParsed,terminalSamples} from './validation';
import * as validation from './validation';
import {runProfile} from './profile';
import type {SessionRef,Target} from './bridge';
vi.mock('../bindings/github.com/sandjaie/KubeSift/internal/desktop',()=>({Service:{TerminalInput:vi.fn().mockResolvedValue(undefined),FixtureMarker:vi.fn().mockResolvedValue({}),RecordValidation:vi.fn().mockResolvedValue(undefined),WatchPods:vi.fn(),StartLogs:vi.fn(),StartTerminal:vi.fn(),StartForward:vi.fn(),ResizeTerminal:vi.fn().mockResolvedValue(undefined)}}));
import {podSelection,reduceEvent,type WorkspaceState} from './state';
it('routes explicit sessions and times only parsed terminal responses',async()=>{
 const a={id:'a',generation:1},b={id:'b',generation:1};
 const state:WorkspaceState={activeTab:'B',tabs:{A:{connection:a,sessions:{sa:{ref:{id:'sa',connection:a},kind:'logs',history:new Uint8Array(),sequence:0,status:'running',truncatedBytes:0}}},B:{connection:b,sessions:{sb:{ref:{id:'sb',connection:b},kind:'logs',history:new Uint8Array(),sequence:0,status:'running',truncatedBytes:0}}}}};
 const event={session:{id:'sa',connection:a},sequence:1,kind:'data',data:btoa('from A')};
 const next=reduceEvent(state,event);
 expect(new TextDecoder().decode(next.tabs.A.sessions.sa.history)).toBe('from A');
 expect(next.tabs.B).toBe(state.tabs.B);
 expect(reduceEvent(next,{...event,sequence:2,session:{id:'sa',connection:{id:'a',generation:2}}})).toBe(next);
 const failure={...event,sequence:2,kind:'error',data:btoa('Session disconnected or failed')};
 const failed=reduceEvent(next,failure);
 const ended=reduceEvent(failed,{...event,sequence:3,kind:'state',data:btoa('ended')});
 expect(ended.tabs.A.sessions.sa.status).toBe('ended');
 expect(ended.observedSessionErrors).toBe(1);
 expect(reduceEvent(ended,failure)).toBe(ended);
 expect(reduceEvent(ended,{...failure,sequence:4,session:{id:'sa',connection:{id:'a',generation:2}}})).toBe(ended);
 const closed={...ended,tabs:{B:ended.tabs.B}};
 expect(closed.observedSessionErrors).toBe(1);
 expect(reduceEvent(closed,{...failure,sequence:4})).toBe(closed);
 expect(reduceEvent(closed,{...event,sequence:2})).toBe(closed);
 const podA={name:'same-name',namespace:'alpha',uid:'pod-a',phase:'Running',ready:true,containers:['fixture'],marker:'A',annotation:'A details'};
 const podB={...podA,namespace:'beta',uid:'pod-b',marker:'B',annotation:'B details'};
 const watch={...state.tabs.A.sessions.sa,kind:'watch',target:{connection:a,namespace:'alpha',name:'',container:''},resources:[podA]};
 const tab={connection:a,namespace:'beta',pods:[],selectedPod:'same-name',container:'fixture',sessions:{watch}};
 expect(podSelection(tab).pods).toEqual([]);
 expect(podSelection(tab).target).toBeNull();
 const selection=podSelection({...tab,pods:[podB]});
 expect(selection.selected).toEqual(podB);
 expect(podSelection({...tab,pods:[podB],container:'old-container'}).target).toBeNull();
 expect(selection.target).toEqual({connection:a,namespace:'beta',name:'same-name',container:'fixture'});
 expect(tab.sessions.watch.resources).toEqual([podA]);
 expect(podSelection({...tab,pods:[podA]}).target).toBeNull();
 expect(podSelection({...tab,sessions:{watch:{...watch,target:{...watch.target,namespace:'beta',connection:b},resources:[podB]}}}).pods).toEqual([]);
 expect(podSelection({...tab,sessions:{watch:{...watch,target:{...watch.target,namespace:'beta',connection:{...a,generation:2}},resources:[podB]}}}).pods).toEqual([]);
 expect(podSelection({...tab,sessions:{watch:{...watch,target:{...watch.target,namespace:'beta'},resources:[podB]}}}).selected).toEqual(podB);
 const ref={id:'terminal',connection:a};terminalReady(ref,true);const measured=terminalSamples(ref);
 for(let i=0;i<20;i++){
  const call=vi.mocked(Service.TerminalInput).mock.calls[i];expect(call[0]).toEqual(ref);
  const command=atob(call[1]??'');const token=command.match(/'([a-f0-9]{32})'/)?.[1];expect(token).toBeTruthy();
  terminalParsed(ref,new TextEncoder().encode(command));await Promise.resolve();await Promise.resolve();
  expect(vi.mocked(Service.TerminalInput).mock.calls.length).toBe(i+1);
  const response=new TextEncoder().encode(`__KS_RTT_${token}__`);terminalParsed(ref,response.subarray(0,12));terminalParsed(ref,response.subarray(12));await Promise.resolve();await Promise.resolve();
 }
 expect(await measured).toHaveLength(20);terminalReady(ref,false);
 vi.useFakeTimers();
 const samples=vi.spyOn(validation,'terminalSamples').mockResolvedValue(Array(20).fill(50));
 try{
  for(const persistentFailure of [false,true]){
  let serial=0;const owned=new Map<string,SessionRef>();const attempts:string[]=[];const statuses:string[]=[];
  const create=(target:Target)=>CancellablePromise.resolve({id:`owned-${++serial}`,connection:target.connection});
  vi.mocked(Service.WatchPods).mockImplementation(create);vi.mocked(Service.StartLogs).mockImplementation(create);vi.mocked(Service.StartTerminal).mockImplementation(create);
  vi.mocked(Service.StartForward).mockImplementation(target=>create(target).then(session=>({session,localPort:12345})));
  const contexts=[{snapshotId:'snapshot',source:'synthetic',name:'a',namespace:'payments',endpoint:'https://127.0.0.1:1'},{snapshotId:'snapshot',source:'synthetic',name:'b',namespace:'payments',endpoint:'https://127.0.0.1:2'}];
  const result=runProfile(contexts,{open:async context=>context.name==='a'?a:b,register:async session=>{owned.set(session.id,session);},stop:async session=>{attempts.push(session.id);if(session.id==='owned-1'&&(persistentFailure||attempts.filter(id=>id===session.id).length===1))throw new Error('injected stop failure');owned.delete(session.id);},state:()=>closed,status:value=>statuses.push(value)}).then(()=>null,error=>error);
  await vi.advanceTimersByTimeAsync(605000);
  expect(await result).toBeNull();
  expect([...owned.keys()]).toEqual(persistentFailure?['owned-1']:[]);
  expect(new Set(attempts).size).toBe(8);
  expect(attempts.filter(id=>id==='owned-1')).toHaveLength(2);
  expect(statuses.join(' ')).toContain('owned-1: injected stop failure');
  expect(vi.mocked(Service.RecordValidation).mock.calls.slice(-1)[0]?.[0]).toMatchObject({phase:4,sessionErrors:1});
  }
 }finally{samples.mockRestore();vi.useRealTimers();}

});
