import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import './index.css'
import { App } from './App'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Tailnet state is pushed by the server's SSE stream from T5 onward, so
      // aggressive refetching here would only duplicate work.
      staleTime: 30_000,
      retry: 1,
    },
  },
})

const root = document.getElementById('root')
if (!root) throw new Error('#root missing from index.html')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
)
