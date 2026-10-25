import { useState } from 'react'
import { Search, X } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router'
import type { Crumb } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * Free-text search in the top bar, within `scope` or site-wide. Below `sm` it
 * is a magnifier button that expands over the whole bar (Esc or X closes it).
 */
export default function SearchBox({ scope }: { scope?: Crumb }) {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [open, setOpen] = useState(false)
  const q = params.get('q') ?? ''
  const label = scope ? `Search ${scope.name}` : 'Search'
  return (
    <>
      {!open && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={label}
          className="ml-auto sm:hidden"
          onClick={() => setOpen(true)}
        >
          <Search />
        </Button>
      )}
      <form
        role="search"
        data-slot="search"
        className={
          open
            ? 'absolute inset-0 z-10 flex items-center gap-2 rounded-[inherit] bg-background px-2 sm:static sm:ml-auto sm:block sm:bg-transparent sm:p-0'
            : 'ml-auto hidden sm:block'
        }
        onSubmit={(e) => {
          e.preventDefault()
          const value = new FormData(e.currentTarget).get('q')?.toString().trim()
          if (value) {
            setOpen(false)
            navigate(`/search?${new URLSearchParams(scope ? { q: value, folder: scope.slug } : { q: value })}`)
          }
        }}
        onKeyDown={(e) => e.key === 'Escape' && setOpen(false)}
      >
        {/* Keyed on q so the field follows Back/Forward between searches. */}
        <Input
          key={q}
          type="search"
          name="q"
          defaultValue={q}
          placeholder={label}
          aria-label={label}
          autoFocus={open}
          enterKeyHint="search"
          className="w-full sm:w-56"
        />
        {open && (
          <Button type="button" variant="ghost" size="icon" aria-label="Close search" className="sm:hidden" onClick={() => setOpen(false)}>
            <X />
          </Button>
        )}
      </form>
    </>
  )
}
