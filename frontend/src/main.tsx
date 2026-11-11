/*
Purpose: Mount the live product interface.
Used by: The frontend HTML entry point and Vite.
Responsibilities: Create the React root and render App with development lifecycle checks.
Important: Normal startup only loads local source summaries; development diagnostics have a separate entry.
*/
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
