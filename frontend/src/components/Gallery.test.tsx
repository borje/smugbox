import { ColumnsPhotoAlbum, MasonryPhotoAlbum, RowsPhotoAlbum } from 'react-photo-album'
import { render } from '@testing-library/react'
import Gallery from '@/components/Gallery'
import { renderAt, setSite } from '@/test/render'
import { photo, summerDetail } from '@/test/fixtures'

vi.mock('react-photo-album', () => ({
  RowsPhotoAlbum: vi.fn(() => null),
  ColumnsPhotoAlbum: vi.fn(() => null),
  MasonryPhotoAlbum: vi.fn(() => null),
}))

const breakpoints = [360, 600, 900, 1200, 1536]

function propsOf(album: unknown) {
  const mock = vi.mocked(album as typeof RowsPhotoAlbum)
  expect(mock).toHaveBeenCalled()
  return mock.mock.lastCall![0] as unknown as Record<string, unknown>
}

function renderWith(gallery?: string) {
  if (gallery) setSite({ title: 'T', theme: { smugbox: 1, name: 'T', dark: false, gallery } })
  renderAt('/a/summer-2026', '/a/:slug', <Gallery photos={summerDetail.photos} />)
}

describe('Gallery layouts', () => {
  beforeEach(() => vi.clearAllMocks())

  it('rows: justified rows', () => {
    renderWith('rows')
    expect(propsOf(RowsPhotoAlbum)).toMatchObject({ spacing: 12, targetRowHeight: 320, padding: 0, breakpoints })
    expect(propsOf(RowsPhotoAlbum).sizes).toBeUndefined()
    expect(ColumnsPhotoAlbum).not.toHaveBeenCalled()
    expect(MasonryPhotoAlbum).not.toHaveBeenCalled()
  })

  it('columns: about one column per 300px, 1–5', () => {
    renderWith('columns')
    const props = propsOf(ColumnsPhotoAlbum)
    expect(props).toMatchObject({ spacing: 20, padding: 0, breakpoints })
    const columns = props.columns as (w: number) => number
    expect(breakpoints.map(columns)).toEqual([1, 2, 3, 4, 5])
    expect(columns(100)).toBe(1)
    expect(columns(3000)).toBe(5)
  })

  it('masonry: 2 / 3 / 4 columns', () => {
    renderWith('masonry')
    const props = propsOf(MasonryPhotoAlbum)
    expect(props).toMatchObject({ spacing: 4, padding: 0, breakpoints })
    const columns = props.columns as (w: number) => number
    expect([639, 640, 999, 1000].map(columns)).toEqual([2, 3, 3, 4])
  })

  it('badges video tiles with their length', () => {
    renderWith('rows')
    type Extras = (p: object, c: { photo: { videoMs?: number } }) => React.ReactNode
    const extras = (propsOf(RowsPhotoAlbum).render as { extras: Extras }).extras
    expect(extras({}, { photo: {} })).toBe(false)
    const { container } = render(<>{extras({}, { photo: { videoMs: 32000 } })}</>)
    expect(container.querySelector('[data-slot="video-badge"]')?.textContent).toBe('0:32')
  })

  it('falls back to rows without site JSON', () => {
    renderWith()
    expect(RowsPhotoAlbum).toHaveBeenCalled()
  })

  it('sizes images by the measured gallery width', () => {
    vi.stubGlobal(
      'ResizeObserver',
      class {
        cb: ResizeObserverCallback
        constructor(cb: ResizeObserverCallback) {
          this.cb = cb
        }
        observe() {
          this.cb([{ contentRect: { width: 1000.4 } } as ResizeObserverEntry], this as unknown as ResizeObserver)
        }
        disconnect() {}
      },
    )
    renderWith('rows')
    expect(propsOf(RowsPhotoAlbum).sizes).toEqual({ size: '1000px' })
    vi.unstubAllGlobals()
  })
})

describe('Gallery lightbox', () => {
  it('plays a video slide from its original', () => {
    const clip = photo('summer-2026', 'v1', { filename: 'CLIP.mp4', mime_type: 'video/mp4', duration_ms: 32000 })
    renderAt('/a/summer-2026?photo=v1', '/a/:slug', <Gallery photos={[clip]} />)
    const video = document.querySelector('video')
    expect(video).not.toBeNull()
    expect(video!.controls).toBe(true)
    expect(video!.getAttribute('poster')).toBe(clip.urls.large)
    expect(video!.querySelector('source')?.getAttribute('src')).toBe(clip.urls.original)
  })
})
