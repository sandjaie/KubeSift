/*
Purpose: Present shared textual status and empty states.
Used by: Product shell and resource views.
Responsibilities: Pair semantic color with readable labels.
Important: Status never relies on color alone.
*/
import type {ViewStatus} from '../workspace/model';
export function StatusBadge({label,tone='neutral'}:{label:string;tone?:'neutral'|'success'|'warning'|'danger'}) {return <span className={`badge ${tone}`}>{label}</span>;}
export function StateMessage({status,title,detail}:{status:ViewStatus;title:string;detail:string}) {return <section className="state-message" role="status"><span className="eyebrow">{status}</span><h2>{title}</h2><p>{detail}</p></section>;}
