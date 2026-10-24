import { useNavigate, useSearchParams } from 'react-router'
import type { Crumb } from '@/api/types'
import { Input } from '@/components/ui/input'

/** Free-text search in the top bar, within `scope` or site-wide. */
export default function SearchBox({ scope }: { scope?: Crumb }) {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const q = params.get('q') ?? ''
  return (
    <form
      role="search"
      data-slot="search"
      className="ml-auto"
      onSubmit={(e) => {
        e.preventDefault()
        const value = new FormData(e.currentTarget).get('q')?.toString().trim()
        if (value) navigate(`/search?${new URLSearchParams(scope ? { q: value, folder: scope.slug } : { q: value })}`)
      }}
    >
      {/* Keyed on q so the field follows Back/Forward between searches. */}
      <Input
        key={q}
        type="search"
        name="q"
        defaultValue={q}
        placeholder={scope ? `Search ${scope.name}` : 'Search'}
        aria-label={scope ? `Search ${scope.name}` : 'Search'}
        className="w-36 sm:w-56"
      />
    </form>
  )
}
