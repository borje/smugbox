// oxlint-disable react/only-export-components -- test helpers, not hot-reloaded

import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from '@/App'

/** Exposes the router location so tests can assert on URL state. */
function LocationProbe() {
  const { pathname, search } = useLocation()
  return <output data-testid="location">{pathname + search}</output>
}

/** Renders the whole app at a route with a fresh, non-retrying query client. */
export function renderApp(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <App />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

export function renderAt(path: string, pattern: string, element: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path={pattern} element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** Stands in for the #smugbox-site JSON the server injects; setup.ts removes it after each test. */
export function setSite(site: unknown) {
  const el = document.createElement('script')
  el.type = 'application/json'
  el.id = 'smugbox-site'
  el.textContent = JSON.stringify(site)
  document.head.append(el)
}
