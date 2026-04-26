import { BrowserRouter, Link, Route, Routes } from 'react-router-dom'
import MenuPage from './pages/MenuPage'
import AdminLogin from './pages/AdminLogin'
import AdminDashboard from './pages/AdminDashboard'
import { AuthProvider } from './contexts/AuthContext'
import { AdminAuthProvider } from './contexts/AdminAuthContext'

function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AdminAuthProvider>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/o/:orgId/t/:tableCode" element={<MenuPage />} />
            <Route path="/admin/login" element={<AdminLogin />} />
            <Route path="/admin" element={<AdminDashboard />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </AdminAuthProvider>
      </AuthProvider>
    </BrowserRouter>
  )
}

function Home() {
  return (
    <div className="home">
      <div className="home-card">
        <h1>Cafe Ordering</h1>
        <p>This is the customer app. To order, scan a table QR code at the cafe.</p>
        <p>
          Customer dev shortcut: <Link to="/o/1/t/t_a7Kx9">/o/1/t/t_a7Kx9</Link> (Tealogy — Table 1)<br />
          Admin / staff dashboard: <Link to="/admin/login">/admin/login</Link>
        </p>
      </div>
    </div>
  )
}

function NotFound() {
  return (
    <div className="home">
      <div className="home-card">
        <h1>Not found</h1>
        <p>That URL doesn't match any page.</p>
        <Link to="/">Home</Link>
      </div>
    </div>
  )
}

export default App
