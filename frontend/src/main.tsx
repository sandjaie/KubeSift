/*
Purpose: Mount the React validation interface.
Used by: The frontend HTML entry point and Vite.
Responsibilities: Create the React root and render App with development lifecycle checks.
Important: Starting React must not implicitly open a Kubernetes connection.
*/
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
