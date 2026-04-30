import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  adminApi,
  assetUrl,
  type AdminMenuItem,
  type AdminOrder,
  type AdminTable,
  type OrgWithStats,
  type StaffRole,
  type StaffRow,
  type StaffWithOrg,
} from '../lib/api'
import { useAdminAuth } from '../contexts/AdminAuthContext'

const TOP_N = 5
const POLL_MS = 5000
const HISTORY_PAGE_SIZE = 10

const ROLE_BADGE: Record<StaffRole, { label: string; cls: string }> = {
  super_admin: { label: 'Super Admin', cls: 'role-super' },
  manager:     { label: 'Branch Admin', cls: 'role-manager' },
  staff:       { label: 'Staff',         cls: 'role-staff' },
}

const PAY_METHODS = ['cash', 'card', 'upi'] as const

type Tab = 'live' | 'history' | 'menu' | 'tables' | 'staff' | 'branches' | 'allstaff'

export default function AdminDashboard() {
  const { state, signOut } = useAdminAuth()
  const nav = useNavigate()

  useEffect(() => {
    if (state.status === 'guest') nav('/admin/login', { replace: true })
  }, [state.status, nav])

  if (state.status === 'unknown') {
    return <div className="status-wrap"><span className="spinner" /> loading…</div>
  }
  if (state.status !== 'authed') {
    return <div className="status-wrap">redirecting…</div>
  }

  return <AuthedDashboard staff={state.staff} signOut={signOut} />
}

function AuthedDashboard({
  staff, signOut,
}: {
  staff: { id: number; email?: string; role: StaffRole; org_id?: number }
  signOut: () => Promise<void>
}) {
  const isSuper = staff.role === 'super_admin'
  const isAdmin = staff.role === 'manager' // branch admin only (super_admin is platform-level)
  // super_admin lands on Branches; branch admin/staff land on Live orders.
  const [tab, setTab] = useState<Tab>(isSuper ? 'branches' : 'live')
  const badge = ROLE_BADGE[staff.role]

  return (
    <div className="admin-shell">
      <header className="admin-bar">
        <div className="brand">
          <h1>Tealogy <span className="dim">{isSuper ? 'Platform' : 'Admin'}</span></h1>
        </div>
        <div className="who">
          <span className={'role-badge ' + badge.cls}>{badge.label}</span>
          <span className="email">{staff.email}</span>
          <button className="btn-ghost" onClick={signOut}>Sign out</button>
        </div>
      </header>

      <nav className="admin-tabs">
        {isSuper ? (
          <>
            <button className={'admin-tab' + (tab === 'branches' ? ' active' : '')} onClick={() => setTab('branches')}>
              Branches
            </button>
            <button className={'admin-tab' + (tab === 'allstaff' ? ' active' : '')} onClick={() => setTab('allstaff')}>
              All staff
            </button>
          </>
        ) : (
          <>
            <button className={'admin-tab' + (tab === 'live' ? ' active' : '')} onClick={() => setTab('live')}>
              Live orders
            </button>
            <button className={'admin-tab' + (tab === 'history' ? ' active' : '')} onClick={() => setTab('history')}>
              History
            </button>
            {isAdmin && (
              <>
                <button className={'admin-tab' + (tab === 'menu' ? ' active' : '')} onClick={() => setTab('menu')}>
                  Menu
                </button>
                <button className={'admin-tab' + (tab === 'tables' ? ' active' : '')} onClick={() => setTab('tables')}>
                  Tables
                </button>
                <button className={'admin-tab' + (tab === 'staff' ? ' active' : '')} onClick={() => setTab('staff')}>
                  Staff
                </button>
              </>
            )}
          </>
        )}
      </nav>

      {tab === 'live'     && !isSuper && <LiveOrdersView />}
      {tab === 'history'  && !isSuper && <HistoryView />}
      {tab === 'menu'     && isAdmin  && <MenuView orgId={staff.org_id ?? 0} />}
      {tab === 'tables'   && isAdmin  && <TablesView orgId={staff.org_id ?? 0} />}
      {tab === 'staff'    && isAdmin  && <StaffView selfId={staff.id} />}
      {tab === 'branches' && isSuper  && <BranchesView />}
      {tab === 'allstaff' && isSuper  && <AllStaffView selfId={staff.id} />}
    </div>
  )
}

// =====================================================================
//  Live orders (two queues + pay modal)
// =====================================================================

function LiveOrdersView() {
  const [orders, setOrders] = useState<AdminOrder[]>([])
  const [error, setError] = useState<string | null>(null)
  const [pendingCode, setPendingCode] = useState<string | null>(null)
  const [payTarget, setPayTarget] = useState<AdminOrder | null>(null)
  const [payMethod, setPayMethod] = useState<typeof PAY_METHODS[number]>('cash')

  const refresh = useCallback(async () => {
    try {
      const res = await adminApi.listActive()
      setOrders(res.orders)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load orders')
    }
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, POLL_MS)
    return () => clearInterval(id)
  }, [refresh])

  const { prep, ready } = useMemo(() => {
    const prep: AdminOrder[] = []
    const ready: AdminOrder[] = []
    for (const o of orders) {
      if (o.status === 'queued' || o.status === 'cooking') prep.push(o)
      else if (o.status === 'prepared') ready.push(o)
    }
    return { prep, ready }
  }, [orders])

  async function move(order: AdminOrder, status: 'cooking' | 'prepared' | 'cancelled') {
    setPendingCode(order.public_code)
    try {
      await adminApi.updateStatus(order.public_code, status)
      await refresh()
    } catch (e: any) {
      setError(e.message ?? 'Status update failed')
    } finally {
      setPendingCode(null)
    }
  }

  async function complete() {
    if (!payTarget) return
    setPendingCode(payTarget.public_code)
    try {
      await adminApi.complete(payTarget.public_code, {
        method: payMethod,
        amount: payTarget.total_amount,
      })
      setPayTarget(null)
      await refresh()
    } catch (e: any) {
      setError(e.message ?? 'Could not complete order')
    } finally {
      setPendingCode(null)
    }
  }

  return (
    <>
      {error && (
        <div className="status-wrap error" style={{ padding: '12px 16px', textAlign: 'left' }}>
          {error}
        </div>
      )}

      <div className="queues">
        <Queue
          title="In the kitchen"
          subtitle="queued + cooking"
          accent="primary"
          orders={prep}
          render={(o) => (
            <OrderCard
              key={o.public_code}
              order={o}
              busy={pendingCode === o.public_code}
              actions={
                o.status === 'queued' ? [
                  { label: 'Start cooking', onClick: () => move(o, 'cooking'), kind: 'primary' },
                  { label: 'Cancel',         onClick: () => move(o, 'cancelled'), kind: 'danger' },
                ] : [
                  { label: 'Mark ready',    onClick: () => move(o, 'prepared'), kind: 'primary' },
                  { label: 'Cancel',         onClick: () => move(o, 'cancelled'), kind: 'danger' },
                ]
              }
            />
          )}
          empty="No active prep. The kitchen is calm."
        />

        <Queue
          title="Ready for pickup"
          subtitle="prepared, waiting for payment"
          accent="accent"
          orders={ready}
          render={(o) => (
            <OrderCard
              key={o.public_code}
              order={o}
              busy={pendingCode === o.public_code}
              actions={[
                { label: 'Mark complete & paid', onClick: () => setPayTarget(o), kind: 'primary' },
                { label: 'Cancel',                onClick: () => move(o, 'cancelled'), kind: 'danger' },
              ]}
            />
          )}
          empty="No orders waiting for pickup."
        />
      </div>

      {payTarget && (
        <div className="modal-backdrop" onClick={() => setPayTarget(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>Mark order paid</h3>
            <p className="muted">
              Order <strong>{payTarget.public_code}</strong> @ {payTarget.table_label}<br />
              Total: <strong>₹{payTarget.total_amount.toFixed(2)}</strong>
            </p>
            <label className="field-label">Payment method</label>
            <div className="seg">
              {PAY_METHODS.map((m) => (
                <button
                  key={m}
                  type="button"
                  className={'seg-btn' + (payMethod === m ? ' active' : '')}
                  onClick={() => setPayMethod(m)}
                >
                  {m.toUpperCase()}
                </button>
              ))}
            </div>
            <div className="modal-actions">
              <button className="btn-secondary" onClick={() => setPayTarget(null)}>Back</button>
              <button className="btn-primary" disabled={pendingCode === payTarget.public_code} onClick={complete}>
                {pendingCode === payTarget.public_code ? 'Saving…' : 'Mark complete & paid'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// =====================================================================
//  History (date-grouped, paginated)
// =====================================================================

function HistoryView() {
  const [orders, setOrders] = useState<AdminOrder[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (off: number) => {
    setLoading(true)
    try {
      const res = await adminApi.listHistory(HISTORY_PAGE_SIZE, off)
      setOrders(res.orders)
      setTotal(res.total)
      setOffset(res.offset)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load history')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load(0) }, [load])

  const grouped = useMemo(() => groupByDate(orders), [orders])
  const page = Math.floor(offset / HISTORY_PAGE_SIZE) + 1
  const pages = Math.max(1, Math.ceil(total / HISTORY_PAGE_SIZE))

  return (
    <div className="history-wrap">
      <div className="history-head">
        <h2>Order history</h2>
        <span className="history-meta">
          {total} order{total === 1 ? '' : 's'} · page {page} of {pages}
        </span>
      </div>

      {error && <div className="status-wrap error" style={{ padding: '8px 0' }}>{error}</div>}

      {loading && orders.length === 0 ? (
        <div className="status-wrap"><span className="spinner" /> loading…</div>
      ) : orders.length === 0 ? (
        <div className="queue-empty">No completed or cancelled orders yet.</div>
      ) : (
        <>
          {grouped.map(([dateKey, list]) => (
            <section key={dateKey} className="history-day">
              <h3>{dateKey}</h3>
              <div className="queue-list">
                {list.map((o) => (
                  <HistoryCard key={o.public_code} order={o} />
                ))}
              </div>
            </section>
          ))}
          <div className="history-pager">
            <button
              className="btn-secondary"
              disabled={loading || offset === 0}
              onClick={() => load(Math.max(0, offset - HISTORY_PAGE_SIZE))}
            >
              ← Newer
            </button>
            <button
              className="btn-secondary"
              disabled={loading || offset + HISTORY_PAGE_SIZE >= total}
              onClick={() => load(offset + HISTORY_PAGE_SIZE)}
            >
              Older →
            </button>
          </div>
        </>
      )}
    </div>
  )
}

function HistoryCard({ order }: { order: AdminOrder }) {
  const t = new Date(order.completed_at ?? order.placed_at)
  const time = t.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit' })
  return (
    <article className={'order-card status-' + order.status}>
      <div className="oc-head">
        <div>
          <span className="oc-table">{order.table_label ?? 'Table'}</span>
          <span className="oc-code">{order.public_code}</span>
        </div>
        <span className="oc-time">{time}</span>
      </div>
      <ul className="oc-items">
        {order.items.map((it) => (
          <li key={it.id}>
            <span className="qty">×{it.quantity}</span>
            <span className="nm">{it.item_name}</span>
            <span className="px">₹{(it.unit_price * it.quantity).toFixed(2)}</span>
          </li>
        ))}
      </ul>
      <div className="oc-foot">
        <div className="oc-total">
          <span className="lbl">Total</span>
          <strong>₹{order.total_amount.toFixed(2)}</strong>
        </div>
        <span className={'history-pill ' + (order.status === 'completed' ? 'ok' : 'cancel')}>
          {order.status === 'completed' ? (order.is_paid ? 'PAID' : 'COMPLETED') : 'CANCELLED'}
        </span>
      </div>
    </article>
  )
}

function groupByDate(orders: AdminOrder[]): [string, AdminOrder[]][] {
  const groups = new Map<string, AdminOrder[]>()
  for (const o of orders) {
    const t = new Date(o.completed_at ?? o.placed_at)
    const key = dateLabel(t)
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key)!.push(o)
  }
  return Array.from(groups.entries())
}

function dateLabel(d: Date): string {
  const today = new Date()
  const dd = new Date(d)
  today.setHours(0, 0, 0, 0)
  dd.setHours(0, 0, 0, 0)
  const days = Math.round((today.getTime() - dd.getTime()) / 86400000)
  if (days === 0) return 'Today'
  if (days === 1) return 'Yesterday'
  return d.toLocaleDateString('en-IN', { day: 'numeric', month: 'short', year: 'numeric' })
}

// =====================================================================
//  Staff (admin-only)
// =====================================================================

function StaffView({ selfId }: { selfId: number }) {
  const [rows, setRows] = useState<StaffRow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<number | null>(null)
  const [showAdd, setShowAdd] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.listStaff()
      setRows(res.staff)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load staff')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function toggleActive(s: StaffRow) {
    setBusy(s.id)
    try {
      await adminApi.updateStaff(s.id, { is_active: !s.is_active })
      await refresh()
    } catch (e: any) {
      setError(e.message ?? 'Update failed')
    } finally {
      setBusy(null)
    }
  }

  async function changeRole(s: StaffRow, role: 'manager' | 'staff') {
    if (role === s.role) return
    setBusy(s.id)
    try {
      await adminApi.updateStaff(s.id, { role })
      await refresh()
    } catch (e: any) {
      setError(e.message ?? 'Update failed')
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="staff-wrap">
      <div className="history-head">
        <h2>Branch staff</h2>
        <button className="btn-primary" style={{ width: 'auto', marginTop: 0 }} onClick={() => setShowAdd(true)}>
          + Add staff
        </button>
      </div>

      {error && <div className="status-wrap error" style={{ padding: '8px 0' }}>{error}</div>}

      {loading ? (
        <div className="status-wrap"><span className="spinner" /> loading…</div>
      ) : (
        <div className="staff-grid">
          {rows.map((s) => {
            const isSelf = s.id === selfId
            return (
              <article key={s.id} className={'staff-card' + (s.is_active ? '' : ' inactive')}>
                <div className="sc-head">
                  <div>
                    <div className="sc-name">{s.name ?? '(no name)'}</div>
                    <div className="sc-email">{s.email}</div>
                    {s.mobile_number && <div className="sc-mobile">{s.mobile_number}</div>}
                  </div>
                  <span className={'role-badge ' + ROLE_BADGE[s.role].cls}>
                    {ROLE_BADGE[s.role].label}
                  </span>
                </div>

                <div className="sc-foot">
                  <div className="seg sm">
                    <button
                      className={'seg-btn' + (s.role === 'manager' ? ' active' : '')}
                      disabled={busy === s.id}
                      onClick={() => changeRole(s, 'manager')}
                    >
                      Admin
                    </button>
                    <button
                      className={'seg-btn' + (s.role === 'staff' ? ' active' : '')}
                      disabled={busy === s.id}
                      onClick={() => changeRole(s, 'staff')}
                    >
                      Staff
                    </button>
                  </div>
                  <button
                    className={s.is_active ? 'act-danger' : 'act-primary'}
                    disabled={busy === s.id || isSelf}
                    title={isSelf ? "You can't disable your own account" : ''}
                    onClick={() => toggleActive(s)}
                  >
                    {s.is_active ? 'Disable' : 'Re-enable'}
                  </button>
                </div>
              </article>
            )
          })}
        </div>
      )}

      {showAdd && (
        <AddStaffModal onClose={() => setShowAdd(false)} onAdded={() => { setShowAdd(false); refresh() }} />
      )}
    </div>
  )
}

function AddStaffModal({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [mobile, setMobile] = useState('')
  const [role, setRole] = useState<'manager' | 'staff'>('staff')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!/^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/.test(email)) {
      setError('Enter a valid email'); return
    }
    if (!name.trim()) { setError('Name is required'); return }
    if (mobile && !/^\+[1-9]\d{7,14}$/.test(mobile)) {
      setError('Phone (optional) must be in international form'); return
    }
    setBusy(true)
    try {
      await adminApi.createStaff({
        email,
        name,
        role,
        mobile_number: mobile || undefined,
      })
      onAdded()
    } catch (e: any) {
      setError(e.message ?? 'Could not add staff')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>Add staff member</h3>
        <p className="muted">They'll be able to sign in at /admin/login with this email.</p>
        <form onSubmit={submit}>
          <label className="field-label">Email</label>
          <input className="field-input" type="email" autoFocus
            value={email} onChange={(e) => setEmail(e.target.value.trim())} placeholder="staff@tealogy.in" />

          <label className="field-label" style={{ marginTop: 12 }}>Name</label>
          <input className="field-input"
            value={name} onChange={(e) => setName(e.target.value)} placeholder="Aisha Patel" />

          <label className="field-label" style={{ marginTop: 12 }}>Phone (optional)</label>
          <input className="field-input" type="tel"
            value={mobile} onChange={(e) => setMobile(e.target.value.trim())} placeholder="+919876543210" />

          <label className="field-label" style={{ marginTop: 12 }}>Role</label>
          <div className="seg">
            <button type="button" className={'seg-btn' + (role === 'staff' ? ' active' : '')} onClick={() => setRole('staff')}>
              Staff
            </button>
            <button type="button" className={'seg-btn' + (role === 'manager' ? ' active' : '')} onClick={() => setRole('manager')}>
              Branch Admin
            </button>
          </div>

          {error && <p className="error-text">{error}</p>}

          <div className="modal-actions">
            <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn-primary" disabled={busy}>
              {busy ? 'Adding…' : 'Add staff'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// =====================================================================
//  Menu (admin-only) — list grouped by category, filter, add/edit, image upload
// =====================================================================

function MenuView({ orgId: _orgId }: { orgId: number }) {
  const [items, setItems] = useState<AdminMenuItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<string | 'ALL'>('ALL')
  const [editing, setEditing] = useState<AdminMenuItem | null>(null)
  const [adding, setAdding] = useState(false)
  const [busy, setBusy] = useState<number | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.listMenu()
      setItems(res.items)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load menu')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const categories = useMemo(() => {
    const set = new Set<string>()
    for (const it of items) if (it.category) set.add(it.category)
    return Array.from(set).sort()
  }, [items])

  const grouped = useMemo(() => {
    const filtered = filter === 'ALL'
      ? items
      : items.filter((it) => (it.category ?? 'Uncategorised') === filter)
    const groups = new Map<string, AdminMenuItem[]>()
    for (const it of filtered) {
      const k = it.category ?? 'Uncategorised'
      if (!groups.has(k)) groups.set(k, [])
      groups.get(k)!.push(it)
    }
    return Array.from(groups.entries())
  }, [items, filter])

  async function toggleAvail(it: AdminMenuItem) {
    setBusy(it.id)
    try {
      await adminApi.updateMenu(it.id, { is_available: !it.is_available })
      await refresh()
    } catch (e: any) { setError(e.message ?? 'Update failed') } finally { setBusy(null) }
  }

  return (
    <div className="staff-wrap">
      <div className="history-head">
        <h2>Menu</h2>
        <button className="btn-primary" style={{ width: 'auto', marginTop: 0 }} onClick={() => setAdding(true)}>
          + Add item
        </button>
      </div>

      {error && <div className="status-wrap error" style={{ padding: '8px 0' }}>{error}</div>}

      <div className="cat-nav" style={{ position: 'static', marginBottom: 16, borderRadius: 999 }}>
        <button
          className={'cat-chip' + (filter === 'ALL' ? ' active' : '')}
          onClick={() => setFilter('ALL')}
        >
          All ({items.length})
        </button>
        {categories.map((c) => (
          <button
            key={c}
            className={'cat-chip' + (filter === c ? ' active' : '')}
            onClick={() => setFilter(c)}
          >
            {c}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="status-wrap"><span className="spinner" /> loading…</div>
      ) : (
        grouped.map(([cat, list]) => (
          <section key={cat} className="history-day">
            <h3>{cat}</h3>
            <div className="menu-admin-grid">
              {list.map((it) => (
                <article key={it.id} className={'menu-admin-card' + (it.is_available ? '' : ' inactive')}>
                  <div className="ma-thumb">
                    {it.image_url ? (
                      <img src={assetUrl(it.image_url)} alt={it.name} />
                    ) : (
                      <span className="ma-placeholder">🍴</span>
                    )}
                  </div>
                  <div className="ma-info">
                    <div className="ma-name">{it.name}</div>
                    {it.description && <div className="ma-desc">{it.description}</div>}
                    <div className="ma-price">₹{it.price.toFixed(2)}</div>
                  </div>
                  <div className="ma-actions">
                    <button className="act-primary" disabled={busy === it.id} onClick={() => setEditing(it)}>
                      Edit
                    </button>
                    <button
                      className={it.is_available ? 'act-danger' : 'act-primary'}
                      disabled={busy === it.id}
                      onClick={() => toggleAvail(it)}
                    >
                      {it.is_available ? 'Hide' : 'Show'}
                    </button>
                  </div>
                </article>
              ))}
            </div>
          </section>
        ))
      )}

      {adding && (
        <MenuItemModal
          item={null}
          onClose={() => setAdding(false)}
          onSaved={async () => { setAdding(false); await refresh() }}
          existingCategories={categories}
        />
      )}
      {editing && (
        <MenuItemModal
          item={editing}
          onClose={() => setEditing(null)}
          onSaved={async () => { setEditing(null); await refresh() }}
          existingCategories={categories}
        />
      )}
    </div>
  )
}

function MenuItemModal({
  item, onClose, onSaved, existingCategories,
}: {
  item: AdminMenuItem | null
  onClose: () => void
  onSaved: () => void
  existingCategories: string[]
}) {
  const isEdit = !!item
  const [name, setName] = useState(item?.name ?? '')
  const [description, setDescription] = useState(item?.description ?? '')
  const [category, setCategory] = useState(item?.category ?? '')
  const [price, setPrice] = useState(item?.price.toString() ?? '')
  const [displayOrder, setDisplayOrder] = useState((item?.display_order ?? 0).toString())
  const [imageUrl, setImageUrl] = useState(item?.image_url ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const p = Number(price)
    if (!name.trim()) { setError('Name required'); return }
    if (!Number.isFinite(p) || p <= 0) { setError('Price must be > 0'); return }
    setBusy(true)
    try {
      const body = {
        name,
        description: description || undefined,
        category: category || undefined,
        price: p,
        display_order: Number(displayOrder) || 0,
        image_url: imageUrl || undefined,
      }
      if (isEdit && item) await adminApi.updateMenu(item.id, body)
      else await adminApi.createMenu(body)
      onSaved()
    } catch (e: any) {
      setError(e.message ?? 'Save failed')
    } finally {
      setBusy(false)
    }
  }

  async function pickFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file || !item) return
    setBusy(true)
    setError(null)
    try {
      const updated = await adminApi.uploadMenuImage(item.id, file)
      setImageUrl(updated.image_url ?? '')
    } catch (e: any) {
      setError(e.message ?? 'Upload failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 480 }}>
        <h3>{isEdit ? 'Edit menu item' : 'Add menu item'}</h3>

        <form onSubmit={submit}>
          <label className="field-label">Name</label>
          <input className="field-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Cappuccino" autoFocus />

          <label className="field-label" style={{ marginTop: 12 }}>Description</label>
          <input className="field-input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Espresso with steamed milk" />

          <label className="field-label" style={{ marginTop: 12 }}>Category</label>
          <input className="field-input" value={category} onChange={(e) => setCategory(e.target.value)} placeholder="Coffee" list="cat-list" />
          <datalist id="cat-list">
            {existingCategories.map((c) => <option key={c} value={c} />)}
          </datalist>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginTop: 12 }}>
            <div>
              <label className="field-label">Price (₹)</label>
              <input className="field-input" type="number" step="0.01" min="0.01" value={price} onChange={(e) => setPrice(e.target.value)} />
            </div>
            <div>
              <label className="field-label">Display order</label>
              <input className="field-input" type="number" value={displayOrder} onChange={(e) => setDisplayOrder(e.target.value)} />
            </div>
          </div>

          <label className="field-label" style={{ marginTop: 12 }}>Image</label>
          {imageUrl && (
            <div className="image-preview">
              <img src={assetUrl(imageUrl)} alt="preview" />
            </div>
          )}
          {isEdit ? (
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <label className="btn-secondary" style={{ width: 'auto', marginTop: 0, padding: '8px 14px', cursor: 'pointer' }}>
                {imageUrl ? 'Replace image' : 'Upload image'}
                <input type="file" accept="image/*" style={{ display: 'none' }} onChange={pickFile} />
              </label>
              {imageUrl && (
                <button type="button" className="act-danger" style={{ padding: '8px 14px', borderRadius: 8 }}
                  onClick={() => setImageUrl('')}>
                  Remove image
                </button>
              )}
            </div>
          ) : (
            <p className="muted" style={{ fontSize: 12, margin: '4px 0 0' }}>
              Save the item first, then you can upload an image.
            </p>
          )}

          {error && <p className="error-text">{error}</p>}

          <div className="modal-actions">
            <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn-primary" disabled={busy}>
              {busy ? 'Saving…' : (isEdit ? 'Save changes' : 'Create')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// =====================================================================
//  Tables (admin-only)
// =====================================================================

function TablesView({ orgId }: { orgId: number }) {
  const [rows, setRows] = useState<AdminTable[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<number | null>(null)
  const [editing, setEditing] = useState<AdminTable | null>(null)
  const [showQR, setShowQR] = useState<AdminTable | null>(null)
  const [creating, setCreating] = useState(false)
  const [newLabel, setNewLabel] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.listTables()
      setRows(res.tables)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load tables')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function create(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setCreating(true)
    try {
      await adminApi.createTable({ label: newLabel || undefined })
      setNewLabel('')
      await refresh()
    } catch (e: any) { setError(e.message ?? 'Create failed') } finally { setCreating(false) }
  }

  async function toggleActive(t: AdminTable) {
    setBusy(t.id)
    try {
      await adminApi.updateTable(t.id, { is_active: !t.is_active })
      await refresh()
    } catch (e: any) { setError(e.message ?? 'Update failed') } finally { setBusy(null) }
  }

  async function saveLabel(t: AdminTable, label: string) {
    setBusy(t.id)
    try {
      await adminApi.updateTable(t.id, { label })
      setEditing(null)
      await refresh()
    } catch (e: any) { setError(e.message ?? 'Update failed') } finally { setBusy(null) }
  }

  return (
    <div className="staff-wrap">
      <div className="history-head">
        <h2>Tables</h2>
      </div>

      <form onSubmit={create} className="add-table-row">
        <input
          className="field-input"
          value={newLabel}
          onChange={(e) => setNewLabel(e.target.value)}
          placeholder='Label (optional, e.g. "Window 3")'
          style={{ flex: 1 }}
        />
        <button type="submit" className="btn-primary" style={{ width: 'auto', marginTop: 0 }} disabled={creating}>
          {creating ? 'Adding…' : '+ Add table'}
        </button>
      </form>

      {error && <div className="status-wrap error" style={{ padding: '8px 0' }}>{error}</div>}

      {loading ? (
        <div className="status-wrap"><span className="spinner" /> loading…</div>
      ) : (
        <div className="staff-grid">
          {rows.map((t) => (
            <article key={t.id} className={'staff-card' + (t.is_active ? '' : ' inactive')}>
              <div className="sc-head">
                <div>
                  {editing?.id === t.id ? (
                    <InlineLabel
                      initial={t.label ?? ''}
                      onCancel={() => setEditing(null)}
                      onSave={(v) => saveLabel(t, v)}
                    />
                  ) : (
                    <div className="sc-name">{t.label ?? '(no label)'}</div>
                  )}
                  <div className="sc-email" style={{ fontFamily: 'ui-monospace, monospace' }}>{t.code}</div>
                </div>
                <span className={'role-badge ' + (t.is_active ? 'role-manager' : 'role-staff')}>
                  {t.is_active ? 'ACTIVE' : 'INACTIVE'}
                </span>
              </div>

              <div className="sc-foot">
                <div style={{ display: 'flex', gap: 6 }}>
                  <button className="act-primary" onClick={() => setShowQR(t)}>Show QR</button>
                  {editing?.id !== t.id && (
                    <button className="act-primary" disabled={busy === t.id} onClick={() => setEditing(t)}>
                      Rename
                    </button>
                  )}
                </div>
                <button
                  className={t.is_active ? 'act-danger' : 'act-primary'}
                  disabled={busy === t.id}
                  onClick={() => toggleActive(t)}
                >
                  {t.is_active ? 'Disable' : 'Re-enable'}
                </button>
              </div>
            </article>
          ))}
        </div>
      )}

      {showQR && <QRModal table={showQR} orgId={orgId} onClose={() => setShowQR(null)} />}
    </div>
  )
}

function InlineLabel({ initial, onSave, onCancel }: {
  initial: string
  onSave: (v: string) => void
  onCancel: () => void
}) {
  const [v, setV] = useState(initial)
  return (
    <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
      <input className="field-input" value={v} onChange={(e) => setV(e.target.value)} autoFocus style={{ padding: 6, fontSize: 14 }} />
      <button className="act-primary" onClick={() => onSave(v.trim())}>Save</button>
      <button className="act-danger" onClick={onCancel}>Cancel</button>
    </div>
  )
}

function QRModal({ table, orgId, onClose }: { table: AdminTable; orgId: number; onClose: () => void }) {
  const url = `${window.location.origin}/o/${orgId}/t/${table.code}`
  const qrSrc = `https://api.qrserver.com/v1/create-qr-code/?data=${encodeURIComponent(url)}&size=300x300&margin=10`
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 380, textAlign: 'center' }}>
        <h3>{table.label ?? table.code}</h3>
        <p className="muted" style={{ fontSize: 13 }}>Print this QR and stick it on the table.</p>
        <img src={qrSrc} alt={`QR for ${table.code}`} style={{ width: '100%', maxWidth: 300, borderRadius: 12, border: '1px solid var(--border)' }} />
        <p className="muted" style={{ fontSize: 12, wordBreak: 'break-all', margin: '12px 0 0' }}>{url}</p>
        <div className="modal-actions" style={{ marginTop: 16 }}>
          <a className="btn-secondary" href={qrSrc} target="_blank" rel="noreferrer" style={{ textAlign: 'center', textDecoration: 'none', flex: 1 }}>
            Open image
          </a>
          <button className="btn-primary" onClick={onClose}>Done</button>
        </div>
      </div>
    </div>
  )
}

// =====================================================================
//  Branches (super_admin only) — list orgs with stats, create new org+manager, edit org
// =====================================================================

function BranchesView() {
  const [orgs, setOrgs] = useState<OrgWithStats[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showAdd, setShowAdd] = useState(false)
  const [editing, setEditing] = useState<OrgWithStats | null>(null)
  const [busy, setBusy] = useState<number | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminApi.listOrgs()
      setOrgs(res.orgs)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load branches')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  async function toggleActive(o: OrgWithStats) {
    setBusy(o.id)
    try {
      await adminApi.updateOrg(o.id, { is_active: !o.is_active })
      await refresh()
    } catch (e: any) {
      setError(e.message ?? 'Update failed')
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="staff-wrap">
      <div className="history-head">
        <h2>Branches</h2>
        <button className="btn-primary" style={{ width: 'auto', marginTop: 0 }} onClick={() => setShowAdd(true)}>
          + Add branch
        </button>
      </div>

      {error && <div className="status-wrap error" style={{ padding: '8px 0' }}>{error}</div>}

      {loading ? (
        <div className="status-wrap"><span className="spinner" /> loading…</div>
      ) : (
        <div className="branches-grid">
          {orgs.map((o) => (
            <article key={o.id} className={'branch-card' + (o.is_active ? '' : ' inactive')}>
              <div className="bc-head">
                <div>
                  <div className="bc-name">{o.name}</div>
                  {o.address && <div className="bc-meta">{o.address}</div>}
                  <div className="bc-meta">
                    {o.contact_phone && <span>📞 {o.contact_phone}</span>}
                    {o.contact_phone && o.contact_email && <span> · </span>}
                    {o.contact_email && <span>✉️ {o.contact_email}</span>}
                  </div>
                </div>
                <span className={'role-badge ' + (o.is_active ? 'role-manager' : 'role-staff')}>
                  {o.is_active ? 'ACTIVE' : 'INACTIVE'}
                </span>
              </div>

              <dl className="bc-stats">
                <Stat label="Staff"  value={o.staff_count} />
                <Stat label="Tables" value={o.table_count} />
                <Stat label="Menu"   value={o.menu_count} />
                <Stat label="Orders" value={o.order_count} />
              </dl>

              <div className="sc-foot">
                <button className="act-primary" disabled={busy === o.id} onClick={() => setEditing(o)}>
                  Edit
                </button>
                <button
                  className={o.is_active ? 'act-danger' : 'act-primary'}
                  disabled={busy === o.id}
                  onClick={() => toggleActive(o)}
                >
                  {o.is_active ? 'Disable' : 'Re-enable'}
                </button>
              </div>
            </article>
          ))}
        </div>
      )}

      {showAdd && <AddBranchModal onClose={() => setShowAdd(false)} onAdded={async () => { setShowAdd(false); await refresh() }} />}
      {editing && <EditBranchModal org={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await refresh() }} />}
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="stat">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}

function AddBranchModal({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const [orgName, setOrgName] = useState('')
  const [address, setAddress] = useState('')
  const [orgPhone, setOrgPhone] = useState('')
  const [orgEmail, setOrgEmail] = useState('')
  const [mgrEmail, setMgrEmail] = useState('')
  const [mgrName, setMgrName] = useState('')
  const [mgrPhone, setMgrPhone] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!orgName.trim()) { setError('Branch name is required'); return }
    if (!/^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/.test(mgrEmail)) {
      setError("Manager's email looks invalid"); return
    }
    if (!mgrName.trim()) { setError("Manager's name is required"); return }
    setBusy(true)
    try {
      await adminApi.createOrgWithManager({
        org: {
          name: orgName,
          address: address || undefined,
          contact_phone: orgPhone || undefined,
          contact_email: orgEmail || undefined,
        },
        manager: {
          email: mgrEmail,
          name: mgrName,
          mobile_number: mgrPhone || undefined,
        },
      })
      onAdded()
    } catch (e: any) {
      setError(e.message ?? 'Could not create branch')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 520 }}>
        <h3>Open a new branch</h3>
        <p className="muted">Creates the branch and its first manager in one step.</p>

        <form onSubmit={submit}>
          <div className="modal-section">
            <div className="modal-section-title">Branch</div>
            <label className="field-label">Name</label>
            <input className="field-input" value={orgName} onChange={(e) => setOrgName(e.target.value)} placeholder="Tealogy — Sector 18" autoFocus />

            <label className="field-label" style={{ marginTop: 10 }}>Address</label>
            <input className="field-input" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="Sector 18, Noida" />

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
              <div>
                <label className="field-label">Contact phone</label>
                <input className="field-input" value={orgPhone} onChange={(e) => setOrgPhone(e.target.value)} placeholder="+91…" />
              </div>
              <div>
                <label className="field-label">Contact email</label>
                <input className="field-input" type="email" value={orgEmail} onChange={(e) => setOrgEmail(e.target.value)} placeholder="hi@…" />
              </div>
            </div>
          </div>

          <div className="modal-section">
            <div className="modal-section-title">First manager (will receive OTPs)</div>
            <label className="field-label">Email</label>
            <input className="field-input" type="email" value={mgrEmail} onChange={(e) => setMgrEmail(e.target.value)} placeholder="manager@tealogy.in" />

            <label className="field-label" style={{ marginTop: 10 }}>Name</label>
            <input className="field-input" value={mgrName} onChange={(e) => setMgrName(e.target.value)} placeholder="Aisha Patel" />

            <label className="field-label" style={{ marginTop: 10 }}>Phone (optional)</label>
            <input className="field-input" value={mgrPhone} onChange={(e) => setMgrPhone(e.target.value)} placeholder="+91…" />
          </div>

          {error && <p className="error-text">{error}</p>}

          <div className="modal-actions">
            <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn-primary" disabled={busy}>
              {busy ? 'Creating…' : 'Open branch'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function EditBranchModal({ org, onClose, onSaved }: {
  org: OrgWithStats
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(org.name)
  const [address, setAddress] = useState(org.address ?? '')
  const [phone, setPhone] = useState(org.contact_phone ?? '')
  const [email, setEmail] = useState(org.contact_email ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name.trim()) { setError('Name required'); return }
    setBusy(true)
    try {
      await adminApi.updateOrg(org.id, {
        name,
        address: address || null,
        contact_phone: phone || null,
        contact_email: email || null,
      })
      onSaved()
    } catch (e: any) {
      setError(e.message ?? 'Save failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 480 }}>
        <h3>Edit branch</h3>
        <form onSubmit={submit}>
          <label className="field-label">Name</label>
          <input className="field-input" value={name} onChange={(e) => setName(e.target.value)} autoFocus />

          <label className="field-label" style={{ marginTop: 10 }}>Address</label>
          <input className="field-input" value={address} onChange={(e) => setAddress(e.target.value)} />

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
            <div>
              <label className="field-label">Contact phone</label>
              <input className="field-input" value={phone} onChange={(e) => setPhone(e.target.value)} />
            </div>
            <div>
              <label className="field-label">Contact email</label>
              <input className="field-input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
          </div>

          {error && <p className="error-text">{error}</p>}

          <div className="modal-actions">
            <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn-primary" disabled={busy}>
              {busy ? 'Saving…' : 'Save changes'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// =====================================================================
//  All staff (super_admin only) — every staff/manager across every org
// =====================================================================

function AllStaffView({ selfId }: { selfId: number }) {
  const [rows, setRows] = useState<StaffWithOrg[]>([])
  const [orgs, setOrgs] = useState<OrgWithStats[]>([])
  const [orgFilter, setOrgFilter] = useState<number | 'ALL'>('ALL')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<number | null>(null)
  const [showAdd, setShowAdd] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const [s, o] = await Promise.all([
        adminApi.listAllStaff(orgFilter === 'ALL' ? undefined : orgFilter),
        adminApi.listOrgs(),
      ])
      setRows(s.staff)
      setOrgs(o.orgs)
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to load staff')
    } finally {
      setLoading(false)
    }
  }, [orgFilter])

  useEffect(() => { refresh() }, [refresh])

  async function changeRole(s: StaffWithOrg, role: 'manager' | 'staff') {
    if (role === s.role) return
    setBusy(s.id)
    try {
      await adminApi.updateAnyStaff(s.id, { role })
      await refresh()
    } catch (e: any) { setError(e.message ?? 'Update failed') } finally { setBusy(null) }
  }
  async function toggleActive(s: StaffWithOrg) {
    setBusy(s.id)
    try {
      await adminApi.updateAnyStaff(s.id, { is_active: !s.is_active })
      await refresh()
    } catch (e: any) { setError(e.message ?? 'Update failed') } finally { setBusy(null) }
  }

  return (
    <div className="staff-wrap">
      <div className="history-head">
        <h2>All staff</h2>
        <button className="btn-primary" style={{ width: 'auto', marginTop: 0 }} onClick={() => setShowAdd(true)}>
          + Add to a branch
        </button>
      </div>

      {error && <div className="status-wrap error" style={{ padding: '8px 0' }}>{error}</div>}

      <div className="cat-nav" style={{ position: 'static', marginBottom: 16 }}>
        <button className={'cat-chip' + (orgFilter === 'ALL' ? ' active' : '')} onClick={() => setOrgFilter('ALL')}>
          All branches
        </button>
        {orgs.map((o) => (
          <button
            key={o.id}
            className={'cat-chip' + (orgFilter === o.id ? ' active' : '')}
            onClick={() => setOrgFilter(o.id)}
          >
            {o.name}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="status-wrap"><span className="spinner" /> loading…</div>
      ) : (
        <div className="staff-grid">
          {rows.map((s) => {
            const isSelf = s.id === selfId
            return (
              <article key={s.id} className={'staff-card' + (s.is_active ? '' : ' inactive')}>
                <div className="sc-head">
                  <div>
                    <div className="sc-name">{s.name ?? '(no name)'}</div>
                    <div className="sc-email">{s.email}</div>
                    {s.org_name && <div className="sc-mobile" style={{ color: 'var(--accent-deep)', fontWeight: 600 }}>📍 {s.org_name}</div>}
                  </div>
                  <span className={'role-badge ' + ROLE_BADGE[s.role].cls}>
                    {ROLE_BADGE[s.role].label}
                  </span>
                </div>

                <div className="sc-foot">
                  <div className="seg sm">
                    <button
                      className={'seg-btn' + (s.role === 'manager' ? ' active' : '')}
                      disabled={busy === s.id}
                      onClick={() => changeRole(s, 'manager')}
                    >
                      Admin
                    </button>
                    <button
                      className={'seg-btn' + (s.role === 'staff' ? ' active' : '')}
                      disabled={busy === s.id}
                      onClick={() => changeRole(s, 'staff')}
                    >
                      Staff
                    </button>
                  </div>
                  <button
                    className={s.is_active ? 'act-danger' : 'act-primary'}
                    disabled={busy === s.id || isSelf}
                    title={isSelf ? "You can't disable your own account" : ''}
                    onClick={() => toggleActive(s)}
                  >
                    {s.is_active ? 'Disable' : 'Re-enable'}
                  </button>
                </div>
              </article>
            )
          })}
        </div>
      )}

      {showAdd && <AddCrossOrgStaffModal orgs={orgs} onClose={() => setShowAdd(false)} onAdded={async () => { setShowAdd(false); await refresh() }} />}
    </div>
  )
}

function AddCrossOrgStaffModal({ orgs, onClose, onAdded }: {
  orgs: OrgWithStats[]
  onClose: () => void
  onAdded: () => void
}) {
  const [orgID, setOrgID] = useState<number | ''>('')
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [mobile, setMobile] = useState('')
  const [role, setRole] = useState<'manager' | 'staff'>('staff')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!orgID) { setError('Pick a branch'); return }
    if (!/^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/.test(email)) {
      setError('Enter a valid email'); return
    }
    if (!name.trim()) { setError('Name required'); return }
    setBusy(true)
    try {
      await adminApi.createStaffInOrg({
        org_id: orgID,
        email, name, role,
        mobile_number: mobile || undefined,
      })
      onAdded()
    } catch (e: any) {
      setError(e.message ?? 'Could not add staff')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>Add staff to a branch</h3>
        <form onSubmit={submit}>
          <label className="field-label">Branch</label>
          <select className="field-input" value={orgID} onChange={(e) => setOrgID(e.target.value ? Number(e.target.value) : '')}>
            <option value="">— select branch —</option>
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </select>

          <label className="field-label" style={{ marginTop: 12 }}>Email</label>
          <input className="field-input" type="email" value={email} onChange={(e) => setEmail(e.target.value.trim())} placeholder="staff@…" />

          <label className="field-label" style={{ marginTop: 12 }}>Name</label>
          <input className="field-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Aisha Patel" />

          <label className="field-label" style={{ marginTop: 12 }}>Phone (optional)</label>
          <input className="field-input" value={mobile} onChange={(e) => setMobile(e.target.value.trim())} placeholder="+91…" />

          <label className="field-label" style={{ marginTop: 12 }}>Role</label>
          <div className="seg">
            <button type="button" className={'seg-btn' + (role === 'staff' ? ' active' : '')} onClick={() => setRole('staff')}>Staff</button>
            <button type="button" className={'seg-btn' + (role === 'manager' ? ' active' : '')} onClick={() => setRole('manager')}>Branch Admin</button>
          </div>

          {error && <p className="error-text">{error}</p>}

          <div className="modal-actions">
            <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn-primary" disabled={busy}>
              {busy ? 'Adding…' : 'Add'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// =====================================================================
//  Shared: Queue + OrderCard
// =====================================================================

function Queue({
  title, subtitle, accent, orders, render, empty,
}: {
  title: string
  subtitle: string
  accent: 'primary' | 'accent'
  orders: AdminOrder[]
  render: (o: AdminOrder) => React.ReactNode
  empty: string
}) {
  const visible = orders.slice(0, TOP_N)
  const overflow = Math.max(0, orders.length - TOP_N)
  return (
    <section className={'queue ' + accent}>
      <header className="queue-head">
        <h2>{title}</h2>
        <span className="queue-meta">
          {orders.length} active · {subtitle}
        </span>
      </header>
      {orders.length === 0 ? (
        <div className="queue-empty">{empty}</div>
      ) : (
        <div className="queue-list">
          {visible.map(render)}
          {overflow > 0 && (
            <div className="queue-overflow">+ {overflow} more in this queue</div>
          )}
        </div>
      )}
    </section>
  )
}

function OrderCard({
  order, busy, actions,
}: {
  order: AdminOrder
  busy: boolean
  actions: { label: string; onClick: () => void; kind: 'primary' | 'danger' }[]
}) {
  const placedAt = new Date(order.placed_at)
  const mins = Math.max(0, Math.floor((Date.now() - placedAt.getTime()) / 60000))
  return (
    <article className={'order-card status-' + order.status}>
      <div className="oc-head">
        <div>
          <span className="oc-table">{order.table_label ?? 'Table'}</span>
          <span className="oc-code">{order.public_code}</span>
        </div>
        <span className="oc-time">{mins}m ago</span>
      </div>
      <ul className="oc-items">
        {order.items.map((it) => (
          <li key={it.id}>
            <span className="qty">×{it.quantity}</span>
            <span className="nm">{it.item_name}</span>
            <span className="px">₹{(it.unit_price * it.quantity).toFixed(2)}</span>
          </li>
        ))}
      </ul>
      <div className="oc-foot">
        <div className="oc-total">
          <span className="lbl">Total</span>
          <strong>₹{order.total_amount.toFixed(2)}</strong>
        </div>
        <span className={`pay-badge ${order.is_paid ? 'pay-badge-paid' : 'pay-badge-unpaid'}`}>
          {order.is_paid ? 'Paid' : 'Not paid'}
        </span>
        <div className="oc-actions">
          {actions.map((a) => (
            <button
              key={a.label}
              disabled={busy}
              onClick={a.onClick}
              className={a.kind === 'primary' ? 'act-primary' : 'act-danger'}
            >
              {a.label}
            </button>
          ))}
        </div>
      </div>
    </article>
  )
}
