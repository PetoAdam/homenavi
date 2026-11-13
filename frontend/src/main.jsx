import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import './index.css'
import App from './App.jsx'
import { createQueryClient } from './state/queryClient'

function startApp() {
  if (import.meta.env.VITE_OTEL_BROWSER_ENABLED === 'true') {
    fetch('/api/telemetry/config', { cache: 'no-store' })
      .then((response) => response.ok ? response.json() : null)
      .then((config) => config?.enabled === true ? import('./services/tracing') : null)
      .then((tracing) => tracing?.initializeBrowserTracing({ enabled: true, sampleRate: import.meta.env.VITE_OTEL_BROWSER_SAMPLE_RATE || 0.1 }))
      .catch(() => console.warn('Browser tracing is unavailable'))
  }

  const queryClient = createQueryClient()

  createRoot(document.getElementById('root')).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <Router>
          <App />
        </Router>
      </QueryClientProvider>
    </StrictMode>,
  )
}

startApp()
