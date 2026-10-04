import { screen, within } from '@testing-library/react'
import { delay, http } from 'msw'
import { server } from '@/test/server'
import { renderApp, setSite } from '@/test/render'

describe('App', () => {
  it('shows the injected site title as the first breadcrumb item', async () => {
    setSite({ title: 'The Granberg Archive', theme: { smugbox: 1, name: 'Noir', dark: true, gallery: 'rows' } })
    renderApp('/f/travel')
    expect(await screen.findByRole('link', { name: 'The Granberg Archive' })).toHaveAttribute('href', '/')
  })

  it('falls back to "Smugbox" without injected site JSON', () => {
    renderApp('/')
    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(within(nav).getByText('Smugbox')).toHaveAttribute('aria-current', 'page')
  })

  it('renders the top bar while a page is still loading', () => {
    server.use(
      http.get('/api/albums/:slug', async () => {
        await delay('infinite')
      }),
    )
    renderApp('/a/summer-2026')
    expect(screen.getByLabelText('Loading album')).toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(within(nav).getByText('Smugbox')).toHaveAttribute('aria-current', 'page')
  })
})
