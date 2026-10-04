import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { api, isApiError } from '@/api/client'
import type { PasswordRequired } from '@/api/types'
import AlbumHero from '@/components/AlbumHero'
import Breadcrumb from '@/components/Breadcrumb'
import Gallery from '@/components/Gallery'
import PasswordGate from '@/components/PasswordGate'
import NotFoundPage from '@/pages/NotFoundPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'

export default function AlbumPage() {
  const { slug = '' } = useParams<{ slug: string }>()
  const queryClient = useQueryClient()
  const { data, isPending, isError, error } = useQuery({
    queryKey: ['album', slug],
    queryFn: () => api.getAlbum(slug),
    retry: false,
  })

  if (isError && isApiError(error, 404)) {
    return <NotFoundPage message="There is no album at this address." />
  }
  // A locked album's 401 still names the album and its folders.
  const locked = isError && isApiError(error, 401, 'password_required') ? (error.body as PasswordRequired) : undefined
  const trail = data ?? locked
  const cover = data?.cover_photo_id ? data.photos.find((p) => p.id === data.cover_photo_id) : undefined
  return (
    <>
      <Breadcrumb crumbs={trail?.breadcrumb} current={trail?.name} />
      {isPending ? (
        <div className="space-y-6" aria-busy="true" aria-label="Loading album">
          <Skeleton className="aspect-[4/3] max-h-[70vh] min-h-[280px] w-full rounded-media sm:aspect-[2/1] lg:aspect-[21/9]" />
          <div className="grid gap-2 sm:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="aspect-[3/2] w-full" />
            ))}
          </div>
        </div>
      ) : locked ? (
        <PasswordGate
          slug={slug}
          name={locked.name}
          count={locked.photo_count}
          onUnlocked={() => queryClient.invalidateQueries({ queryKey: ['album', slug] })}
        />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load the album</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : (
        <>
          <AlbumHero album={data} cover={cover} />
          <Gallery photos={data.photos} />
        </>
      )}
    </>
  )
}
