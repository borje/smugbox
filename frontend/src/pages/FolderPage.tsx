import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router'
import { api, isApiError } from '@/api/client'
import Breadcrumb from '@/components/Breadcrumb'
import ChildGrid, { ChildGridSkeleton } from '@/components/ChildGrid'
import NotFoundPage from '@/pages/NotFoundPage'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

export default function FolderPage() {
  const { slug = '' } = useParams<{ slug: string }>()
  const { data, isPending, isError, error } = useQuery({
    queryKey: ['folder', slug],
    queryFn: () => api.getFolder(slug),
    retry: false,
  })

  if (isError && isApiError(error, 404)) {
    return <NotFoundPage message="There is no folder at this address." />
  }
  return (
    <>
      <Breadcrumb crumbs={data?.breadcrumb} current={data?.name} />
      {isPending ? (
        <ChildGridSkeleton label="Loading folder" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load the folder</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : (
        <ChildGrid folders={data.folders} albums={data.albums} emptyMessage="This folder is empty." />
      )}
    </>
  )
}
