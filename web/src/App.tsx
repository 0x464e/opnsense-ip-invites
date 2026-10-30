import { Link, Route, Routes } from 'react-router-dom'
import AdminPage from './AdminPage'
import InvitePage from './InvitePage'
import { Brand, Footer } from './components'

export default function App() {
  return (
    <Routes>
      <Route path="/i/:token" element={<InvitePage />} />
      <Route path="/admin/*" element={<AdminPage />} />
      <Route path="*" element={<Landing />} />
    </Routes>
  )
}
function Landing() {
  return (
    <div className="flex min-h-dvh flex-col">
      <header className="mx-auto w-full max-w-6xl px-6 py-7">
        <Brand />
      </header>
      <main className="flex flex-1 items-center justify-center px-6">
        <div className="max-w-md text-center">
          <p className="eyebrow mb-4">Access by invitation</p>
          <h1 className="text-3xl font-semibold tracking-tight">Your invitation starts here.</h1>
          <p className="mt-4 text-sm leading-relaxed text-muted">
            Open the invitation link you were given to authorize your connection.
          </p>
          {/* Let the reverse proxy authenticate the admin document request. */}
          <Link
            reloadDocument
            className="mt-7 inline-block text-sm font-medium text-leaf underline underline-offset-4"
            to="/admin"
          >
            Administrator access
          </Link>
        </div>
      </main>
      <Footer />
    </div>
  )
}
