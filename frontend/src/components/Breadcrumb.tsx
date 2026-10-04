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
import { getSite } from '@/lib/site'

/**
 * The top bar on every page: site name, then the folders above the current
 * page (`crumbs`, links), then the page itself (`current`, plain text).
 * With neither, the site name is the current page (root, loading, errors).
 */
export default function Breadcrumb({ crumbs = [], current }: { crumbs?: Crumb[]; current?: string }) {
  const title = getSite().title
  const links =
    current === undefined ? [] : [{ name: title, to: '/' }, ...crumbs.map((c) => ({ name: c.name, to: `/f/${c.slug}` }))]
  return (
    <Root aria-label="Breadcrumb" className="rounded-pill">
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
    </Root>
  )
}
