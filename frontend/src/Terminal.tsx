/*
Purpose: Render one fixed container terminal and its byte stream.
Used by: The validation Sessions dock.
Responsibilities: Attach xterm, send bounded input and resize requests, and acknowledge parsed output.
Important: Keep this component mounted across tab switches; disable escape-driven links and clipboard actions and never replay commands.
*/
import {memo,useEffect,useRef,useState} from 'react';
import {Terminal as XTerm} from '@xterm/xterm';
import {FitAddon} from '@xterm/addon-fit';
import {terminalReady,terminalParsed} from './validation';
import '@xterm/xterm/css/xterm.css';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import {activateSession,acknowledge,encodeBytes,eventBytes,onSessionEvent,type SessionRef} from './bridge';

export default memo(function TerminalView({session,label}:{session:SessionRef;label:string}){
 const host=useRef<HTMLDivElement>(null);const [problem,setProblem]=useState('');
 const id=session.id,connectionId=session.connection.id,generation=session.connection.generation;
 useEffect(()=>{
  if(!host.current)return;let disposed=false;let sequence=0;
  const ref={id,connection:{id:connectionId,generation}};
  const terminal=new XTerm({scrollback:1000,allowProposedApi:false,windowOptions:{},linkHandler:{activate:()=>{}},theme:{background:'#102323',foreground:'#e0eeee'},fontSize:13});
  const fit=new FitAddon();terminal.loadAddon(fit);terminal.open(host.current);
  const blockedLinks=terminal.parser.registerOscHandler(8,()=>true);const blockedClipboard=terminal.parser.registerOscHandler(52,()=>true);
  const report=(message:string)=>{if(!disposed)setProblem(message);};
  const input=terminal.onData(data=>{void Service.TerminalInput(ref,encodeBytes(new TextEncoder().encode(data))).catch(()=>report('Terminal input was not accepted. Check session state or input backpressure.'));});
  const binary=terminal.onBinary(data=>{void Service.TerminalInput(ref,btoa(data)).catch(()=>report('Terminal byte input was not accepted.'));});
  const resize=()=>{if(!host.current||host.current.clientWidth===0||host.current.clientHeight===0)return;fit.fit();if(terminal.cols>0&&terminal.rows>0)void Service.ResizeTerminal(ref,terminal.cols,terminal.rows).catch(()=>report('Terminal resize was not accepted.'));};
  const observer=new ResizeObserver(resize);observer.observe(host.current);
  const off=onSessionEvent(event=>{if(event.session.id!==id||event.session.connection.id!==connectionId||event.session.connection.generation!==generation||event.kind!=='data'||event.sequence<=sequence)return;sequence=event.sequence;const bytes=eventBytes(event);terminal.write(bytes,()=>{terminalParsed(ref,bytes);void acknowledge(ref,event.sequence).catch(()=>report('Terminal output flow control ended. Close or restart this session.'));});});
  resize();void activateSession(ref).then(()=>{terminalReady(ref,true);terminal.focus();}).catch(()=>report('Terminal could not start or was cancelled.'));
  return ()=>{disposed=true;terminalReady(ref,false);off();observer.disconnect();input.dispose();binary.dispose();blockedLinks.dispose();blockedClipboard.dispose();terminal.dispose();};
 },[id,connectionId,generation]);
 return <div className="terminal"><p>{label}</p>{problem&&<p role="alert">{problem}</p>}<div ref={host} className="terminal-surface" aria-label={`Terminal ${label}`}/></div>;
});
