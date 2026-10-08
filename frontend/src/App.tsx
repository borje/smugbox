import { Outlet, Route, Routes } from 'react-router'
import AlbumListPage from '@/pages/AlbumListPage'
import AlbumPage from '@/pages/AlbumPage'
import FolderPage from '@/pages/FolderPage'
import NotFoundPage from '@/pages/NotFoundPage'

// No header: each page starts with the breadcrumb, which is the top bar.
function Layout() {
  return (
    <main data-slot="page" className="mx-auto max-w-screen-2xl space-y-6 px-4 py-8">
      <Outlet />
    </main>
  )
}

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<AlbumListPage />} />
        <Route path="a/:slug" element={<AlbumPage />} />
        <Route path="f/:slug" element={<FolderPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
