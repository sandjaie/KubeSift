/*
Purpose: Prove that tab switches and delayed events cannot mix session output.
Used by: The single frontend routing test in Vitest.
Responsibilities: Route A output while B is active and reject stale-generation or closed-tab events.
Important: Keep this a behavior test; do not expand it into an exhaustive UI or snapshot suite.
*/
import {it,expect} from 'vitest';
import {reduceEvent,type WorkspaceState} from './state';
it('routes by session and ignores stale generations',()=>{
 const a={id:'a',generation:1},b={id:'b',generation:1};
 const state:WorkspaceState={activeTab:'B',tabs:{A:{connection:a,sessions:{sa:{ref:{id:'sa',connection:a},kind:'logs',history:new Uint8Array(),sequence:0,status:'running',truncatedBytes:0}}},B:{connection:b,sessions:{sb:{ref:{id:'sb',connection:b},kind:'logs',history:new Uint8Array(),sequence:0,status:'running',truncatedBytes:0}}}}};
 const event={session:{id:'sa',connection:a},sequence:1,kind:'data',data:btoa('from A')};
 const next=reduceEvent(state,event);
 expect(new TextDecoder().decode(next.tabs.A.sessions.sa.history)).toBe('from A');
 expect(next.tabs.B).toBe(state.tabs.B);
 expect(reduceEvent(next,{...event,sequence:2,session:{id:'sa',connection:{id:'a',generation:2}}})).toBe(next);
 const closed={...next,tabs:{B:next.tabs.B}};
 expect(reduceEvent(closed,{...event,sequence:2})).toBe(closed);
});
