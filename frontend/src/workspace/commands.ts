/*
Purpose: Define shared product commands and keyboard handling.
Used by: Workspace and command palette.
Responsibilities: Keep labels, availability and shortcuts in one registry.
Important: Editable, terminal, modal and composing input never triggers global commands.
*/
export const commands=[{id:'palette',label:'Commands',shortcut:'⌘/Ctrl K',key:'k',shift:false,target:false},{id:'logs',label:'Open logs preview',shortcut:'⌘/Ctrl ⇧ L',key:'l',shift:true,target:true},{id:'terminal',label:'Terminal preview',shortcut:'⌘/Ctrl ⇧ T',key:'t',shift:true,target:true},{id:'forward',label:'Forward preview',shortcut:'⌘/Ctrl ⇧ P',key:'p',shift:true,target:true}] as const;
export type CommandId=typeof commands[number]['id'];
export function commandForKey(event:KeyboardEvent):CommandId|null{if(event.isComposing||event.altKey||!(event.metaKey||event.ctrlKey))return null;const el=event.target;if(el instanceof Element&&(el.closest('input,textarea,select,[contenteditable="true"],.xterm,dialog,[role="dialog"]')))return null;if(document.querySelector('dialog[open]'))return null;return commands.find(c=>c.key===event.key.toLowerCase()&&c.shift===event.shiftKey)?.id??null;}
