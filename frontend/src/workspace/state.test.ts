/*
Purpose: Protect synthetic workspace target isolation.
Used by: Vitest checks.
Responsibilities: Exercise independent tabs, frozen reviews and session ownership.
Important: All resources are synthetic; no desktop bridge is imported.
*/
import {expect,it} from 'vitest';
import {createPreview} from '../preview/fixtures';
import {initialUiState,reduceUi} from './state';
it('keeps tab navigation and review targets isolated',()=>{
 const preview=createPreview();const [a,b]=preview.tabs;const ra=preview.resources.find(r=>r.key.clusterId===a.id)!;const rb=preview.resources.find(r=>r.key.clusterId===b.id)!;
 let s=reduceUi(initialUiState(),{type:'preview',...preview});
 s=reduceUi(s,{type:'tab',id:a.id,patch:{scope:{kind:'namespace',name:ra.key.namespace!},selected:ra.key,filter:'checkout',detail:'logs',scroll:{list:32,detail:64}}});
 s=reduceUi(s,{type:'review',action:'delete',target:ra.key});
 const original=s.tabs[0];const review=s.review!;
 s=reduceUi(s,{type:'selectTab',id:b.id});
 s=reduceUi(s,{type:'tab',id:b.id,patch:{scope:{kind:'namespace',name:rb.key.namespace!},selected:rb.key,filter:'api',detail:'yaml',scroll:{list:9,detail:8}}});
 expect(s.tabs[0]).toEqual(original);expect(s.review?.target).toEqual(ra.key);
 s=reduceUi(s,{type:'closeTab',id:a.id});s=reduceUi(s,{type:'confirmReview',id:review.id});
 expect(s.resources.some(r=>r.key.uid===rb.key.uid)).toBe(true);expect(s.review).toBeNull();
});
