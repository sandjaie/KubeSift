/*
Purpose: Present accessible explicit confirmations with frozen targets.
Used by: Preview operation and tab-close reviews, command palette.
Responsibilities: Contain focus, cancel with Escape and restore initiating focus.
Important: Initial focus is cancel; opening a review never executes an action.
*/
import {useEffect,useRef,type ReactNode} from 'react';
import {identity,type ActionReview} from '../workspace/model';
export function Modal({title,children,onCancel}:{title:string;children:ReactNode;onCancel:()=>void}){
 const dialog=useRef<HTMLDialogElement>(null);const cancel=useRef<HTMLButtonElement>(null);
 useEffect(()=>{const previous=document.activeElement as HTMLElement|null;const node=dialog.current;node?.showModal();cancel.current?.focus();return ()=>{node?.close();if(previous?.isConnected)previous.focus();};},[]);
 return <dialog ref={dialog} aria-label={title} onCancel={e=>{e.preventDefault();onCancel();}} onKeyDown={e=>{if(e.key!=='Tab')return;const nodes=Array.from(e.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled),input,select,textarea,[tabindex="0"]'));const first=nodes[0],last=nodes[nodes.length-1];if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}}}><header className="modal-header"><h2>{title}</h2><button ref={cancel} onClick={onCancel}>Cancel</button></header>{children}</dialog>;
}
export function ReviewDialog({review,onConfirm,onCancel}:{review:ActionReview;onConfirm:()=>void;onCancel:()=>void}){return <Modal title={`Simulate ${review.action}`} onCancel={onCancel}><p>{review.description}</p><p className="frozen-target">{identity(review.target)}</p><p className="muted">This target is fixed. Navigation cannot retarget this review.</p><button className="primary" onClick={onConfirm}>Simulate {review.action}</button></Modal>;}
