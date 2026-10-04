import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import './index.css'
import App from './App.tsx'

// Browser bar tint (mobile) follows the theme's background. The tokens are
// usually oklch; a 1×1 canvas turns any CSS colour into rgb, which every
// browser accepts in theme-color. The theme stylesheet is in <head> before
// this module runs, so the computed value is already the theme's.
function setThemeColor() {
  const ctx = document.createElement('canvas').getContext('2d')
  if (!ctx) return
  ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--background')
  ctx.fillRect(0, 0, 1, 1)
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
  const meta = document.createElement('meta')
  meta.name = 'theme-color'
  meta.content = '#' + [r, g, b].map((n) => n.toString(16).padStart(2, '0')).join('')
  document.head.append(meta)
}
setThemeColor()

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: false } },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
