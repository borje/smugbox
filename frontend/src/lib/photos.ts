import type { Photo as GalleryPhoto } from 'react-photo-album'
import type { SlideImage, SlideVideo } from 'yet-another-react-lightbox'
import type { Photo } from '@/api/types'
import { formatDateTime } from '@/lib/format'

const srcSetVariants = ['thumb', 'small', 'medium', 'large'] as const

export interface SrcSetEntry {
  src: string
  width: number
  height: number
}

/** One entry per distinct variant size; a variant equal in size to a
 *  smaller one is dropped (the backend never upscales). */
export function buildSrcSet(photo: Photo): SrcSetEntry[] {
  const out: SrcSetEntry[] = []
  for (const variant of srcSetVariants) {
    const size = photo.sizes[variant]
    if (out.some((e) => e.width === size.width)) continue
    out.push({ src: photo.urls[variant], ...size })
  }
  return out
}

export function isVideo(photo: Photo): boolean {
  return photo.mime_type.startsWith('video/')
}

/** `m:ss`, or `h:mm:ss` from an hour up. */
export function formatDuration(ms: number): string {
  const s = Math.round(ms / 1000)
  const mmss = `${Math.floor(s / 60) % 60}:${String(s % 60).padStart(2, '0')}`
  return s >= 3600 ? `${Math.floor(s / 3600)}:${mmss.padStart(5, '0')}` : mmss
}

export type GalleryItem = GalleryPhoto & {
  id: string
  filename: string
  /** Set for videos: their length, or 0 when unknown. */
  videoMs?: number
  caption?: string
  takenAt?: string
  exif?: Record<string, string>
}

export function toGalleryPhoto(photo: Photo): GalleryItem {
  return {
    key: photo.id,
    id: photo.id,
    src: photo.urls.medium,
    width: photo.width,
    height: photo.height,
    srcSet: buildSrcSet(photo),
    alt: photo.title || photo.filename,
    title: photo.title,
    filename: photo.filename,
    caption: photo.caption,
    takenAt: photo.taken_at,
    exif: photo.exif,
    videoMs: isVideo(photo) ? (photo.duration_ms ?? 0) : undefined,
  }
}

export type GallerySlide = (SlideImage | SlideVideo) & {
  download: { url: string; filename: string }
}

export function toSlide(photo: Photo): GallerySlide {
  const download = { url: photo.urls.download, filename: photo.filename }
  if (isVideo(photo)) {
    return {
      type: 'video',
      poster: photo.urls.large,
      width: photo.width,
      height: photo.height,
      sources: [{ src: photo.urls.original, type: photo.mime_type }],
      download,
    }
  }
  return {
    src: photo.urls.large,
    width: photo.width,
    height: photo.height,
    srcSet: buildSrcSet(photo),
    alt: photo.title || photo.filename,
    download,
  }
}

export interface DetailRow {
  label: string
  value: string
}

/** Rows for the info panel, in display order; fields the plugin did not send are skipped. */
export function photoDetails(photo: Photo): DetailRow[] {
  const exif = photo.exif ?? {}
  const camera = [exif.make, exif.model].filter(Boolean).join(' ')
  // Lightroom's "exposure" is shutter and aperture combined; fall back to the parts.
  const exposure = exif.exposure || [exif.shutter_speed, exif.aperture].filter(Boolean).join(' at ')
  const rows: Array<[string, string | undefined]> = [
    ['Taken', formatDateTime(photo.taken_at)],
    ['Camera', camera],
    ['Lens', exif.lens],
    ['Focal length', exif.focal_length],
    ['Exposure', exposure],
    ['ISO', exif.iso],
    ['Dimensions', `${photo.width} × ${photo.height}`],
    ['Filename', photo.filename],
  ]
  return rows.filter((r): r is [string, string] => !!r[1] && r[1].trim() !== '').map(([label, value]) => ({ label, value }))
}
