import { Fragment } from 'react'
import { Link } from 'react-router'
import type { Crumb } from '@/api/types'
import {
  Breadcrumb as Root,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import SearchBox from '@/components/SearchBox'
import { getSite } from '@/lib/site'

/**
 * The top bar on every page: site name, then the folders above the current
 * page (`crumbs`, links), then the page itself (`current`, plain text).
 * With neither, the site name is the current page (root, loading, errors).
 * The search box sits at the right end and searches `scope`.
 */
export default function Breadcrumb({
  crumbs = [],
  current,
  scope,
}: {
  crumbs?: Crumb[]
  current?: string
  /** The folder the search box searches; omitted = the whole site. */
  scope?: Crumb
}) {
  const title = getSite().title
  const links =
    current === undefined ? [] : [{ name: title, to: '/' }, ...crumbs.map((c) => ({ name: c.name, to: `/f/${c.slug}` }))]
  return (
    <Root aria-label="Breadcrumb" className="flex items-center gap-4 rounded-pill">
      <BreadcrumbList>
        {links.map((item) => (
          <Fragment key={item.to}>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link to={item.to}>{item.name}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
          </Fragment>
        ))}
        <BreadcrumbItem>
          {/* shadcn marks the page role="link"; the current page is plain text. */}
          <BreadcrumbPage role={undefined} aria-disabled={undefined}>
            {current ?? title}
          </BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
      <SearchBox scope={scope} />
    </Root>
  )
}
