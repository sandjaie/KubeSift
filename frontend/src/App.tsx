import {useEffect,useState} from 'react';
import {Service} from '../bindings/github.com/sandjaie/KubeSift/internal/desktop';
export default function App(){const [status,setStatus]=useState('connecting'); useEffect(()=>{Service.Ping().then(setStatus).catch(()=>setStatus('Desktop bridge unavailable'));},[]);return <main><h1>KubeSift · Phase 1A</h1><p role="status">{status}</p></main>}
