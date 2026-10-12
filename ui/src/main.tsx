import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'

import './index.css'
import { App } from './App'
import { connectivity } from './lib/connectivity'
import { onMutationError, onQueryError } from './lib/session'

const queryClient: QueryClient = new QueryClient({
  // Every request reports whether it reached the server, which is what the
  // offline screen keys on (an HTTP error still means we are online).
  queryCache: new QueryCache({
    onError: (error, query): void => onQueryError(queryClient, error, query.queryKey),
    onSuccess: connectivity.ok,
  }),
  mutationCache: new MutationCache({ onError: onMutationError }),
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
