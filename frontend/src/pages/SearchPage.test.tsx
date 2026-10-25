import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderApp } from '@/test/render'

describe('SearchPage', () => {
  it('searches from the top bar and opens a photo in its album', async () => {
    renderApp('/')
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Search' }), 'iceland{Enter}')
    expect(screen.getByTestId('location')).toHaveTextContent('/search?q=iceland')
    await userEvent.click(await screen.findByRole('link', { name: 'Photos (1)' }))
    const photo = await screen.findByRole('link', { name: 'Glacier' })
    expect(photo).toHaveAttribute('href', '/a/iceland?photo=i1')
    expect(screen.getByRole('searchbox', { name: 'Search' })).toHaveValue('iceland')
  })

  it('expands from a magnifier button on small screens and closes again', async () => {
    renderApp('/')
    await userEvent.click(await screen.findByRole('button', { name: 'Search' }))
    expect(screen.queryByRole('button', { name: 'Search' })).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Close search' }))
    expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument()
  })

  it('switches between result types', async () => {
    renderApp('/search?q=iceland')
    const tabs = await screen.findByRole('navigation', { name: 'Result types' })
    expect(within(tabs).getByRole('link', { name: 'Albums (1)' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: 'Iceland' })).toHaveAttribute('href', '/a/iceland')
    expect(screen.queryByRole('link', { name: 'Glacier' })).toBeNull()
    await userEvent.click(within(tabs).getByRole('link', { name: 'Photos (1)' }))
    expect(await screen.findByRole('link', { name: 'Glacier' })).toBeInTheDocument()
  })

  it('opens the first tab with hits', async () => {
    renderApp('/search?q=glacier')
    const tabs = await screen.findByRole('navigation', { name: 'Result types' })
    expect(within(tabs).getByRole('link', { name: 'Photos (1)' })).toHaveAttribute('aria-current', 'page')
  })

  it('searches the current folder, with a way out to the whole site', async () => {
    renderApp('/f/travel')
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Search Travel' }), 'iceland{Enter}')
    expect(screen.getByTestId('location')).toHaveTextContent('/search?q=iceland&folder=travel')
    expect(await screen.findByRole('link', { name: 'Search everywhere' })).toBeInTheDocument()
    const nav = screen.getByRole('navigation', { name: 'Breadcrumb' })
    expect(within(nav).getByRole('link', { name: 'Travel' })).toHaveAttribute('href', '/f/travel')
    expect(within(nav).getByText('Search')).toHaveAttribute('aria-current', 'page')
    // The box on the results page keeps searching the same folder.
    expect(screen.getByRole('searchbox', { name: 'Search Travel' })).toHaveValue('iceland')
    await userEvent.click(screen.getByRole('link', { name: 'Albums (1)' }))
    expect(screen.getByTestId('location')).toHaveTextContent('/search?q=iceland&folder=travel&tab=albums')
    await userEvent.click(screen.getByRole('link', { name: 'Search everywhere' }))
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/search\?q=iceland$/)
    expect(await screen.findByRole('link', { name: 'Albums (1)' })).toHaveAttribute('aria-current', 'page')
    expect(screen.queryByRole('link', { name: 'Search everywhere' })).toBeNull()
  })

  it('searches an album’s folder from the album page', async () => {
    renderApp('/a/iceland')
    expect(await screen.findByRole('searchbox', { name: 'Search Travel' })).toBeInTheDocument()
  })

  it('searches the whole site from a top-level album', async () => {
    renderApp('/a/summer-2026')
    expect(await screen.findByRole('searchbox', { name: 'Search' })).toBeInTheDocument()
  })

  it('shows not found for an unknown folder', async () => {
    renderApp('/search?q=x&folder=nope')
    expect(await screen.findByText(/There is no folder at this address/)).toBeInTheDocument()
  })

  it('says when nothing matches', async () => {
    renderApp('/search?q=zzz')
    expect(await screen.findByText('No albums match “zzz”.')).toBeInTheDocument()
  })
})
