/*
Purpose: Define supported product keyboard commands in one place.
Used by: App and its command palette.
Responsibilities: Offer cluster selection while guarding text, modal and composing input.
Important: No simulated operations or terminal commands exist in this product registry.
*/
export const commands=[{id:'palette',label:'Commands',shortcut:'⌘/Ctrl K',key:'k',shift:false,target:false},{id:'clusters',label:'Choose cluster',shortcut:'',key:'',shift:false,target:false}] as const;
export type CommandId=typeof commands[number]['id'];
export function commandForKey(event:KeyboardEvent):CommandId|null{if(event.isComposing||event.altKey||!(event.metaKey||event.ctrlKey))return null;const el=event.target;if(el instanceof Element&&el.closest('input,textarea,select,[contenteditable="true"],.xterm,dialog,[role="dialog"]'))return null;if(document.querySelector('dialog[open]'))return null;return commands.find(c=>c.key===event.key.toLowerCase()&&c.shift===event.shiftKey)?.id??null;}
