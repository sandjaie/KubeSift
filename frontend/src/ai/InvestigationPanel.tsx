/*
Purpose: Explain AI availability honestly from any product page.
Used by: App's global right-side panel.
Responsibilities: Provide an accessible close control and explain future live investigation.
Important: No data collection, provider configuration or model request occurs here.
*/
export function InvestigationPanel({onClose}:{onClose:()=>void}){return <aside className="ai-panel" aria-label="AI investigation"><header><h2>AI investigation</h2><button aria-label="Close AI panel" onClick={onClose}>×</button></header><div className="ai-content"><h3>Live investigation is not available yet.</h3><p>You can browse your clusters while this panel stays open. AI integration is planned for a later milestone.</p><p className="muted">No cluster data is collected or sent to an AI provider. There is no active model or sample diagnosis.</p></div></aside>;}
