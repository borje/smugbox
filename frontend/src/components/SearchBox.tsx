import { useEffect, useRef, useState } from 'react'
import { Search, X } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router'
import type { Crumb } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

/**
 * Free-text search in the top bar, within `scope` or site-wide: a magnifier
 * button. From `sm` the field appears beside it on hover or click; below `sm`
 * a click expands it over the whole bar. Esc, X or leaving the field closes it.
 */
export default function SearchBox({ scope }: { scope?: Crumb }) {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [open, setOpen] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const q = params.get('q') ?? ''
  const label = scope ? `Search ${scope.name}` : 'Search'
  // Focus once the field is displayed; it is display: none while closed.
  useEffect(() => {
    if (open) input.current?.focus()
  }, [open])
  return (
    <form
      role="search"
      data-slot="search"
      className={cn(
        'group ml-auto flex items-center gap-2',
        open &&
          'absolute inset-0 z-10 rounded-[inherit] bg-background px-2 max-sm:animate-in max-sm:fade-in max-sm:slide-in-from-right-8 sm:static sm:bg-transparent sm:p-0',
      )}
      onSubmit={(e) => {
        e.preventDefault()
        const value = new FormData(e.currentTarget).get('q')?.toString().trim()
        if (value) {
          input.current?.blur()
          setOpen(false)
          navigate(`/search?${new URLSearchParams(scope ? { q: value, folder: scope.slug } : { q: value })}`)
        }
      }}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          input.current?.blur()
          setOpen(false)
        }
      }}
      onBlur={(e) => !e.currentTarget.contains(e.relatedTarget) && setOpen(false)}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={label}
        className={cn(open && 'hidden sm:inline-flex')}
        onClick={() => (open ? input.current?.focus() : setOpen(true))}
      >
        <Search />
      </Button>
      {/* Keyed on q so the field follows Back/Forward between searches. */}
      <Input
        ref={input}
        key={q}
        type="search"
        name="q"
        defaultValue={q}
        placeholder={label}
        aria-label={label}
        enterKeyHint="search"
        // From sm the field grows out of zero width, so it slides in from the right.
        className={cn(
          'w-full sm:w-56 sm:transition-all sm:duration-300 motion-reduce:transition-none',
          !open &&
            'hidden sm:block sm:w-0 sm:px-0 sm:opacity-0 sm:group-hover:w-56 sm:group-hover:px-2.5 sm:group-hover:opacity-100 sm:group-focus-within:w-56 sm:group-focus-within:px-2.5 sm:group-focus-within:opacity-100',
        )}
      />
      {open && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Close search"
          className="sm:hidden"
          onClick={() => setOpen(false)}
        >
          <X />
        </Button>
      )}
    </form>
  )
}
