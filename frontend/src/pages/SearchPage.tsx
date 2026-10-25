import { useQuery } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router'
import { api, isApiError } from '@/api/client'
import Breadcrumb from '@/components/Breadcrumb'
import ChildGrid, { ChildGridSkeleton, cardGrid } from '@/components/ChildGrid'
import FadeImage from '@/components/FadeImage'
import NotFoundPage from '@/pages/NotFoundPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { formatDate } from '@/lib/format'
import { cn } from '@/lib/utils'

// Album hits first: a search for an album name often also matches many
// photos (by keyword), which would otherwise bury it behind a tab.
const tabs = [
  { id: 'albums', label: 'Albums' },
  { id: 'photos', label: 'Photos' },
] as const
type Tab = (typeof tabs)[number]['id']

/** /search URL; `folder` limits it to that folder, `tab` picks the result type. */
function searchPath(q: string, folder: string, tab?: Tab) {
  const params = new URLSearchParams({ q })
  if (folder) params.set('folder', folder)
  if (tab) params.set('tab', tab)
  return `/search?${params}`
}

export default function SearchPage() {
  const [params] = useSearchParams()
  const q = params.get('q')?.trim() ?? ''
  const folder = params.get('folder') ?? ''
  const { data, isPending, isError, error } = useQuery({
    queryKey: ['search', q, folder],
    queryFn: () => api.search(q, folder || undefined),
    enabled: q !== '',
    retry: false,
  })

  const scope = data?.breadcrumb?.at(-1)

  if (isError && isApiError(error, 404)) {
    return <NotFoundPage message="There is no folder at this address." />
  }

  const tabParam = params.get('tab')
  // Without an explicit tab, open the first one that has hits.
  const tab: Tab = tabs.some((t) => t.id === tabParam)
    ? (tabParam as Tab)
    : (tabs.find((t) => data && data[t.id].length > 0)?.id ?? tabs[0].id)

  return (
    <>
      <Breadcrumb crumbs={data?.breadcrumb} current="Search" scope={scope} />
      {q === '' ? (
        <p data-slot="empty-state" className="py-24 text-center text-muted-foreground">
          Search photo titles, captions and keywords, and album names.
        </p>
      ) : isPending ? (
        <ChildGridSkeleton label="Searching" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Search failed</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : (
        <>
          {scope && (
            // The breadcrumb names the folder; this is the way out of it.
            <p data-slot="search-scope" className="text-sm">
              <Link to={searchPath(q, '')} className="text-muted-foreground underline underline-offset-4 hover:text-foreground">
                Search everywhere
              </Link>
            </p>
          )}
          <nav data-slot="search-tabs" aria-label="Result types" className="flex gap-5">
            {tabs.map((t) => (
              <Link
                key={t.id}
                to={searchPath(q, folder, t.id)}
                replace
                aria-current={t.id === tab ? 'page' : undefined}
                className={cn(
                  'py-1 text-sm text-muted-foreground underline-offset-8 hover:text-foreground',
                  t.id === tab && 'text-foreground underline',
                )}
              >
                {t.label} ({data[t.id].length})
              </Link>
            ))}
          </nav>
          {tab === 'photos' ? (
            data.photos.length === 0 ? (
              <p data-slot="empty-state" className="py-24 text-center text-muted-foreground">
                No photos match “{q}”.
              </p>
            ) : (
              // Photo hits reuse the album-card slots inside child-grid, so every
              // theme's grid and card styling applies to them too.
              <div data-slot="child-grid" className={cardGrid}>
                {data.photos.map((p) => (
                  <Link
                    key={p.id}
                    data-slot="album-card"
                    to={`/a/${p.album_slug}?photo=${p.id}`}
                    className="group block rounded-panel focus:outline-none"
                  >
                    <div
                      data-slot="album-card-media"
                      className="aspect-[var(--cover-aspect)] w-full overflow-hidden rounded-media bg-muted group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-ring"
                    >
                      <FadeImage
                        src={p.urls.thumb}
                        alt=""
                        className="size-full object-cover transition-[transform,filter] duration-300 ease-out group-hover:scale-[1.03] group-hover:brightness-110"
                      />
                    </div>
                    <div data-slot="album-card-body" className="space-y-0.5 px-1 pt-3">
                      <h2 data-slot="album-card-title" className="truncate font-medium leading-tight">
                        {p.title || p.filename}
                      </h2>
                      {p.taken_at && (
                        <p data-slot="album-card-meta" className="text-sm text-muted-foreground">
                          {formatDate(p.taken_at)}
                        </p>
                      )}
                    </div>
                  </Link>
                ))}
              </div>
            )
          ) : (
            <ChildGrid
              folders={[]}
              albums={data.albums}
              emptyMessage={`No albums match “${q}”.`}
            />
          )}
        </>
      )}
    </>
  )
}
