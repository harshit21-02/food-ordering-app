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
    <div className="landing">
      <header className="landing-nav">
        <div className="landing-brand">
          <span className="lb-mark">Mesa</span>
          <span className="lb-tag">order at your table</span>
        </div>
        <Link to="/admin/login" className="lnav-link">Cafe sign in</Link>
      </header>

      <section className="landing-hero">
        <div className="landing-hero-text">
          <span className="eyebrow">Order at your table</span>
          <h1>Skip the queue.<br />Order from your phone.</h1>
          <p>Scan the QR sticker on your table, browse the menu, and the kitchen sees your order the moment you tap "Save".</p>
          <div className="landing-cta">
            <Link to="/o/1/t/t_a7Kx9" className="btn-primary-lg">Try the demo</Link>
            <Link to="/admin/login" className="btn-ghost-lg">Cafe owner?</Link>
          </div>
        </div>
        <div className="landing-hero-art" aria-hidden="true">
          <div className="phone-mock">
            <div className="phone-mock-screen">
              <div className="pms-bar">Tealogy Cafe</div>
              <div className="pms-item">
                <span>☕ Cappuccino</span>
                <span>₹180</span>
              </div>
              <div className="pms-item">
                <span>🥪 Sandwich</span>
                <span>₹120</span>
              </div>
              <div className="pms-item">
                <span>🍵 Masala Chai</span>
                <span>₹39</span>
              </div>
              <div className="pms-stepper">−&nbsp;&nbsp;3&nbsp;&nbsp;+</div>
              <div className="pms-cta">Save changes · ₹339</div>
            </div>
          </div>
        </div>
      </section>

      <section className="landing-how">
        <div className="lh-inner">
          <h2>How it works</h2>
          <div className="how-grid">
            <div className="how-step">
              <span className="how-icon">📱</span>
              <h3>Scan</h3>
              <p>Point your phone at the QR on your table. The cafe's menu opens in your browser — no app to install.</p>
            </div>
            <div className="how-step">
              <span className="how-icon">☕</span>
              <h3>Order</h3>
              <p>Tap items, watch your total update, place the order. Friends at the same table can add to it from their own phones.</p>
            </div>
            <div className="how-step">
              <span className="how-icon">🔔</span>
              <h3>Receive</h3>
              <p>Watch the status pill flip Queued → Cooking → Ready. Pay with the staff when it lands.</p>
            </div>
          </div>
        </div>
      </section>

      <section className="landing-features">
        <div className="features-text">
          <span className="eyebrow alt">For cafes</span>
          <h2>Run your cafe smarter</h2>
          <p>One dashboard for the kitchen, the floor, and the books. No printers, no shouting orders, no waitlist scribbles.</p>
        </div>
        <div className="features-grid">
          <div className="feature">
            <span>👨‍🍳</span>
            <h4>Live kitchen queue</h4>
            <p>Two columns: prep and ready. Tap a card to move it forward. Customers see the status change in real time.</p>
          </div>
          <div className="feature">
            <span>👥</span>
            <h4>Multi-customer tables</h4>
            <p>Two friends can each scan and add to the same shared order. Everyone's items show up on one bill.</p>
          </div>
          <div className="feature">
            <span>🍽️</span>
            <h4>Menu under your control</h4>
            <p>Add items, upload photos, hide what's out of stock. Changes reach the customer's phone within seconds.</p>
          </div>
          <div className="feature">
            <span>🔐</span>
            <h4>Email-OTP login</h4>
            <p>No passwords for staff or customers. Different roles for branch admin, kitchen staff, and platform owner.</p>
          </div>
          <div className="feature">
            <span>💳</span>
            <h4>Cash, UPI, card</h4>
            <p>Mark each order paid with the method used. Per-day, per-method totals available in history.</p>
          </div>
          <div className="feature">
            <span>📊</span>
            <h4>Date-grouped history</h4>
            <p>Every completed and cancelled order, paginated and grouped by day. Audit trail for free.</p>
          </div>
        </div>
      </section>

      <section className="landing-cta-bar">
        <div>
          <h2>Want it for your cafe?</h2>
          <p>Get a branch + manager set up in one step. Email <a href="mailto:hello@mesa.app">hello@mesa.app</a> to get on-boarded.</p>
        </div>
        <Link to="/admin/login" className="btn-primary-lg">Open admin sign-in →</Link>
      </section>

      <footer className="landing-foot">
        <div className="foot-shortcuts">
          Demo links —
          <Link to="/o/1/t/t_a7Kx9"> /o/1/t/t_a7Kx9</Link>
          <span> · </span>
          <Link to="/admin/login">/admin/login</Link>
        </div>
        <div className="foot-tech">Built with Go, React, and Postgres.</div>
      </footer>
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
