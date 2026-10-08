export type GalleryLayout = 'rows' | 'columns' | 'masonry'

/** What the server injects into index.html as #smugbox-site (internal/web). */
export interface Site {
  title: string
  theme: { gallery: GalleryLayout }
}

const fallback: Site = { title: 'Smugbox', theme: { gallery: 'rows' } }

/** The site title and theme manifest, or neutral defaults when absent (tests, a bare build). */
export function getSite(): Site {
  try {
    const text = document.getElementById('smugbox-site')?.textContent
    return text ? { ...fallback, ...JSON.parse(text) } : fallback
  } catch {
    return fallback
  }
}
