import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import Breadcrumb from '@/components/Breadcrumb'
import ChildGrid, { ChildGridSkeleton } from '@/components/ChildGrid'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

export default function AlbumListPage() {
  const { data, isPending, isError, error } = useQuery({ queryKey: ['albums'], queryFn: api.listRoot })

  return (
    <>
      <Breadcrumb />
      {isPending ? (
        <ChildGridSkeleton label="Loading albums" />
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load albums</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : (
        <ChildGrid folders={data.folders} albums={data.albums} />
      )}
    </>
  )
}
