import { Link } from 'react-router'
import { ImageOff, Lock } from 'lucide-react'
import FadeImage from '@/components/FadeImage'
import type { AlbumSummary } from '@/api/types'
import { formatDateRange, photoCount } from '@/lib/format'

// Editorial card: the cover is the card, the text sits below it on the page
// background. Hover zooms the image slightly; focus outlines the cover.
// box-shadow on the cover is left to the theme.
export default function AlbumCard({ album }: { album: AlbumSummary }) {
  const dates = formatDateRange(album.taken_from, album.taken_to)
  return (
    <Link data-slot="album-card" to={`/a/${album.slug}`} className="group block rounded-panel focus:outline-none" aria-label={album.name}>
      <div data-slot="album-card-media" className="relative aspect-[var(--cover-aspect)] w-full overflow-hidden rounded-media bg-muted transition-shadow group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-ring">
        {album.cover_url ? (
          <FadeImage
            src={album.cover_url}
            alt=""
            className={
              'size-full object-cover transition-[transform,filter] duration-300 ease-out group-hover:scale-[1.03] group-hover:brightness-110 ' +
              (album.locked ? 'scale-110 blur-md' : '')
            }
          />
        ) : (
          <div className="flex size-full items-center justify-center text-muted-foreground">
            <ImageOff className="size-8" aria-hidden="true" />
          </div>
        )}
        {/* A scrim over the photo: white in every theme. */}
        {album.locked && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/30 text-white">
            <Lock className="size-10 drop-shadow" aria-label="Password protected" role="img" />
          </div>
        )}
      </div>
      <div data-slot="album-card-body" className="space-y-0.5 px-1 pt-3">
        <h2 data-slot="album-card-title" className="truncate font-medium leading-tight">
          {album.name}
        </h2>
        <p data-slot="album-card-meta" className="text-sm text-muted-foreground">
          {photoCount(album.photo_count)}
          {dates && <> · {dates}</>}
        </p>
      </div>
    </Link>
  )
}
