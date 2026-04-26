import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, assetUrl, type ContextResponse, type MenuItem, type Order } from '../lib/api'
import { useAuth } from '../contexts/AuthContext'
import LoginScreen from '../components/LoginScreen'

type Loadable<T> =
  | { kind: 'loading' }
  | { kind: 'ok'; data: T }
  | { kind: 'error'; message: string }

const CATEGORY_ORDER = [
  'Desi Chai',
  'Flavoured Tea',
  'Iced Tea',
  'Coffee',
  'Cold Coffee',
  'Milk',
  'Ice Crush',
  'Cheesecake Milk Shake',
  'Shakes',
  'Fruit Shakes',
  'Coolers',
  'Tea-Snacks',
  'Sandwich',
  'Maggi',
  'Desi',
  'Momos',
  'Corn',
  'Burger',
  'Rice Bowl',
]

// Lightweight icon per category — no asset pipeline needed.
const CATEGORY_ICON: Record<string, string> = {
  'Desi Chai': '🍵',
  'Flavoured Tea': '🌿',
  'Iced Tea': '🧊',
  'Coffee': '☕',
  'Cold Coffee': '🧋',
  'Milk': '🥛',
  'Ice Crush': '🍧',
  'Cheesecake Milk Shake': '🍰',
  'Shakes': '🥤',
  'Fruit Shakes': '🍓',
  'Coolers': '🍹',
  'Tea-Snacks': '🍞',
  'Sandwich': '🥪',
  'Maggi': '🍜',
  'Desi': '🥘',
  'Momos': '🥟',
  'Corn': '🌽',
  'Burger': '🍔',
  'Rice Bowl': '🍚',
}

const STATUS_LABELS: Record<string, string> = {
  queued: 'Queued',
  cooking: 'Cooking',
  prepared: 'Ready',
  completed: 'Completed',
  cancelled: 'Cancelled',
}

// Convert "Desi Chai" → "cat-desi-chai" for use in hrefs/IDs.
const slug = (s: string) => 'cat-' + s.toLowerCase().replace(/[^a-z0-9]+/g, '-')

export default function MenuPage() {
  const { orgId, tableCode } = useParams<{ orgId: string; tableCode: string }>()
  const { state, signOut } = useAuth()

  const [ctx, setCtx] = useState<Loadable<ContextResponse>>({ kind: 'loading' })
  const [menu, setMenu] = useState<Loadable<MenuItem[]>>({ kind: 'loading' })
  const [activeOrder, setActiveOrder] = useState<Order | null>(null)
  const [cart, setCart] = useState<Map<number, number>>(new Map())
  const [placing, setPlacing] = useState(false)
  const [placeError, setPlaceError] = useState<string | null>(null)
  const [justPlaced, setJustPlaced] = useState(false)

  useEffect(() => {
    if (!orgId || !tableCode) return
    api.getContext(orgId, tableCode)
      .then((data) => setCtx({ kind: 'ok', data }))
      .catch((e: Error) => setCtx({ kind: 'error', message: e.message }))
  }, [orgId, tableCode])

  useEffect(() => {
    if (!orgId || !tableCode) return
    if (state.status !== 'authed') return
    api.getMenu(orgId)
      .then((data) => setMenu({ kind: 'ok', data: data.items }))
      .catch((e: Error) => setMenu({ kind: 'error', message: e.message }))
    api.getActiveOrder(orgId, tableCode)
      .then((order) => setActiveOrder(order))
      .catch(() => setActiveOrder(null))
  }, [orgId, tableCode, state.status])

  const grouped = useMemo(() => {
    if (menu.kind !== 'ok') return null
    const groups = new Map<string, MenuItem[]>()
    for (const item of menu.data) {
      const key = item.category ?? 'Uncategorised'
      const list = groups.get(key) ?? []
      list.push(item)
      groups.set(key, list)
    }
    const order = (cat: string) => {
      const i = CATEGORY_ORDER.indexOf(cat)
      return i === -1 ? Number.MAX_SAFE_INTEGER : i
    }
    return Array.from(groups.entries()).sort(([a], [b]) => order(a) - order(b))
  }, [menu])

  const placedByMenuItemID = useMemo(() => {
    const map = new Map<number, number>()
    if (!activeOrder) return map
    for (const it of activeOrder.items) {
      if (!it.menu_item_id) continue
      map.set(it.menu_item_id, (map.get(it.menu_item_id) ?? 0) + it.quantity)
    }
    return map
  }, [activeOrder])

  const cartLines = useMemo(() => {
    if (menu.kind !== 'ok') return []
    const byID = new Map(menu.data.map((m) => [m.id, m]))
    return Array.from(cart.entries())
      .map(([id, delta]) => ({ item: byID.get(id), delta }))
      .filter((x): x is { item: MenuItem; delta: number } => !!x.item && x.delta !== 0)
  }, [cart, menu])

  const cartTotal = cartLines.reduce((s, l) => s + l.item.price * l.delta, 0)
  const cartCount = cartLines.reduce((s, l) => s + Math.abs(l.delta), 0)

  function changeQty(itemID: number, delta: number, placed: number) {
    setCart((prev) => {
      const next = new Map(prev)
      const cur = next.get(itemID) ?? 0
      const proposed = cur + delta
      const clamped = Math.max(-placed, proposed)
      if (clamped === 0) next.delete(itemID)
      else next.set(itemID, clamped)
      return next
    })
  }

  async function place() {
    if (!ctx || ctx.kind !== 'ok') return
    if (cartLines.length === 0) return
    setPlacing(true)
    setPlaceError(null)
    try {
      const order = await api.placeOrAppend({
        org_id: ctx.data.org.id,
        table_id: ctx.data.table.id,
        items: cartLines.map((l) => ({ menu_item_id: l.item.id, delta: l.delta })),
      })
      setActiveOrder(order.items.length > 0 ? order : null)
      setCart(new Map())
      setJustPlaced(true)
      setTimeout(() => setJustPlaced(false), 4000)
      window.scrollTo({ top: 0, behavior: 'smooth' })
    } catch (e: any) {
      setPlaceError(e.message ?? 'Failed to update order')
    } finally {
      setPlacing(false)
    }
  }

  // ----- render -----

  if (ctx.kind === 'loading' || state.status === 'unknown') {
    return (
      <div className="status-wrap">
        <span className="spinner" /> loading…
      </div>
    )
  }

  if (ctx.kind === 'error') {
    return (
      <div className="status-wrap error">
        could not load this table — {ctx.message}
      </div>
    )
  }

  if (state.status !== 'authed') {
    return <LoginScreen orgName={ctx.data.org.name} />
  }

  const customerLabel = state.customer.email
    ?? state.customer.mobile_number
    ?? `Customer #${state.customer.id}`

  return (
    <div className="app-shell">
      <div className="brand-bar">
        <h1>{ctx.data.org.name}</h1>
        <div className="tagline">Yaar Mera Kulhad</div>
        <div className="since">Since 2018</div>
      </div>

      <div className="sub-bar">
        <div className="meta">
          <span className="table-tag">
            {ctx.data.table.label ?? `Table ${ctx.data.table.code}`}
          </span>
          <span style={{ color: 'var(--muted-2)', fontSize: 12 }}>
            {customerLabel}
          </span>
        </div>
        <button className="btn-ghost" onClick={signOut}>Sign out</button>
      </div>

      {grouped && grouped.length > 0 && (
        <nav className="cat-nav">
          {grouped.map(([cat]) => (
            <a key={cat} href={`#${slug(cat)}`} className="cat-chip">
              <span className="ico">{CATEGORY_ICON[cat] ?? '🍴'}</span>
              {cat}
            </a>
          ))}
        </nav>
      )}

      {justPlaced && (
        <div className="placed-toast">
          <span style={{ fontSize: 18 }}>✓</span>
          Order updated. Kitchen has been notified.
        </div>
      )}

      {activeOrder && <RunningPanel order={activeOrder} />}

      {menu.kind === 'loading' && (
        <div className="status-wrap"><span className="spinner" /> loading menu…</div>
      )}
      {menu.kind === 'error' && (
        <div className="status-wrap error">menu failed to load — {menu.message}</div>
      )}

      {grouped && grouped.map(([category, items]) => (
        <section key={category} id={slug(category)} className="section">
          <div className="section-title">
            <span className="ico">{CATEGORY_ICON[category] ?? '🍴'}</span>
            <h2>{category}</h2>
            <span className="rule" />
          </div>
          <div className="cards">
            {items.map((it) => {
              const placed = placedByMenuItemID.get(it.id) ?? 0
              const delta = cart.get(it.id) ?? 0
              const total = placed + delta
              return (
                <article className="card" key={it.id}>
                  <div className="thumb">
                    {it.image_url ? (
                      <img src={assetUrl(it.image_url)} alt={it.name} />
                    ) : (
                      CATEGORY_ICON[category] ?? '🍴'
                    )}
                  </div>
                  <div className="info">
                    <div className="name">{it.name}</div>
                    {it.description && <div className="desc">{it.description}</div>}
                    <div className="price">
                      <span className="currency">₹</span>{it.price.toFixed(2)}
                    </div>
                  </div>
                  <CardQty
                    total={total}
                    onMinus={() => changeQty(it.id, -1, placed)}
                    onPlus={() => changeQty(it.id, +1, placed)}
                  />
                </article>
              )
            })}
          </div>
        </section>
      ))}

      {cartCount > 0 && (
        <div className="cart-bar">
          <div className="summary">
            <span className="small">
              {activeOrder ? 'Update your order' : 'Place order'}
            </span>
            {cartCount} change{cartCount > 1 ? 's' : ''}
            {' · '}
            {cartTotal >= 0 ? '+' : '−'}₹{Math.abs(cartTotal).toFixed(2)}
          </div>
          <button onClick={place} disabled={placing}>
            {placing
              ? 'Saving…'
              : activeOrder
                ? 'Save changes'
                : 'Place order'}
          </button>
        </div>
      )}

      {placeError && (
        <div className="status-wrap error" style={{ paddingTop: 16 }}>
          {placeError}
        </div>
      )}
    </div>
  )
}

function CardQty({
  total,
  onMinus,
  onPlus,
}: {
  total: number
  onMinus: () => void
  onPlus: () => void
}) {
  return (
    <div className={`qty${total > 0 ? ' has-items' : ''}`}>
      <button onClick={onMinus} aria-label="Decrease">−</button>
      <span className="count">{total}</span>
      <button onClick={onPlus} aria-label="Increase">+</button>
    </div>
  )
}

function RunningPanel({ order }: { order: Order }) {
  return (
    <div className="running-panel">
      <div className="head">
        <div className="left">
          <span className="status-pill">{STATUS_LABELS[order.status] ?? order.status}</span>
          Current order
        </div>
        <span className="code">{order.public_code}</span>
      </div>

      <ul className="lines">
        {order.items.map((it) => (
          <li key={it.id}>
            <span className="qty-px">×{it.quantity}</span>
            <span className="nm">{it.item_name}</span>
            <span className="px">₹{(it.unit_price * it.quantity).toFixed(2)}</span>
          </li>
        ))}
      </ul>

      <div className="total">
        <span className="label">Running total</span>
        <span>₹{order.total_amount.toFixed(2)}</span>
      </div>

      <div className="hint">Add or remove items below — changes apply when you tap "Save changes".</div>
    </div>
  )
}
