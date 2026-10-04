import type { AlbumSummary, FolderSummary } from '@/api/types'
import AlbumCard from '@/components/AlbumCard'
import FolderCard from '@/components/FolderCard'
import { Skeleton } from '@/components/ui/skeleton'

const grid = 'grid grid-cols-[repeat(auto-fill,minmax(min(100%,var(--card-grid-min)),1fr))] gap-[var(--card-gap)]'

// Folders first, then albums — both already arrive newest-content-first
// from the backend. Shared by the root page and folder pages.
export default function ChildGrid({
  folders,
  albums,
  emptyMessage = 'No albums have been published yet.',
}: {
  folders: FolderSummary[]
  albums: AlbumSummary[]
  emptyMessage?: string
}) {
  if (folders.length === 0 && albums.length === 0) {
    return (
      <p data-slot="empty-state" className="py-24 text-center text-muted-foreground">
        {emptyMessage}
      </p>
    )
  }
  return (
    <div data-slot="child-grid" className={grid}>
      {folders.map((folder) => (
        <FolderCard key={folder.slug} folder={folder} />
      ))}
      {albums.map((album) => (
        <AlbumCard key={album.slug} album={album} />
      ))}
    </div>
  )
}

/** Placeholder cards in the same grid while a listing loads. */
export function ChildGridSkeleton({ label }: { label: string }) {
  return (
    <div data-slot="child-grid" className={grid} aria-busy="true" aria-label={label}>
      {Array.from({ length: 6 }).map((_, i) => (
        <div key={i} className="space-y-3">
          <Skeleton className="aspect-[var(--cover-aspect)] w-full rounded-media" />
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-3 w-1/3" />
        </div>
      ))}
    </div>
  )
}
