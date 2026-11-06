/*
Purpose: Mount the React validation interface.
Used by: The frontend HTML entry point and Vite.
Responsibilities: Create the React root and render App with development lifecycle checks.
Important: Normal startup never opens a connection; an explicit fixture-guarded validation environment may run the packaged profile.
*/
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
