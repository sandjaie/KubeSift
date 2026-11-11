/*
Purpose: Offer discoverable target-scoped commands.
Used by: Workspace.
Responsibilities: Show shortcuts and explain disabled actions.
Important: Palette invokes the same callbacks as resource buttons.
*/
import {commands,type CommandId} from './commands';
import {Modal} from '../ui/ReviewDialog';
export function CommandPalette({available,onCommand,onClose}:{available:boolean;onCommand:(id:CommandId)=>void;onClose:()=>void}){return <Modal title="Workspace commands" onCancel={onClose}><p className="muted">Choose an existing context. Resource operations are not available yet.</p><div className="command-list">{commands.filter(c=>c.id!=='palette').map(c=><button key={c.id} disabled={c.target&&!available} onClick={()=>{onClose();onCommand(c.id);}}>{c.label}<kbd>{c.shortcut}</kbd></button>)}</div></Modal>;}
