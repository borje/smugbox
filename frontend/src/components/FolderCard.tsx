import { Link } from 'react-router'
import { ImageOff } from 'lucide-react'
import FadeImage from '@/components/FadeImage'
import type { FolderSummary } from '@/api/types'

// Same card shape as AlbumCard: cover is the card, name sits below it.
// No stats, no lock overlay — folders never carry a password.
export default function FolderCard({ folder }: { folder: FolderSummary }) {
  return (
    <Link data-slot="folder-card" to={`/f/${folder.slug}`} className="group block rounded-panel focus:outline-none" aria-label={folder.name}>
      <div data-slot="folder-card-media" className="relative aspect-[var(--cover-aspect)] w-full overflow-hidden rounded-media bg-muted transition-shadow group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-ring">
        {folder.cover_url ? (
          <FadeImage
            src={folder.cover_url}
            alt=""
            className="size-full object-cover transition-[transform,filter] duration-300 ease-out group-hover:scale-[1.03] group-hover:brightness-110"
          />
        ) : (
          <div className="flex size-full items-center justify-center text-muted-foreground">
            <ImageOff className="size-8" aria-hidden="true" />
          </div>
        )}
      </div>
      <div data-slot="folder-card-body" className="space-y-0.5 px-1 pt-3">
        <h2 data-slot="folder-card-title" className="truncate font-medium leading-tight">
          {folder.name}
        </h2>
      </div>
    </Link>
  )
}
