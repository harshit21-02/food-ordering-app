// In dev, VITE_API_URL is empty → API_BASE becomes "/api/v1" and Vite proxies
// to the local backend. In production, VITE_API_URL is the deployed backend
// origin (e.g. "https://cafe-backend.onrender.com"), making API_BASE absolute.
const API_ORIGIN = (import.meta.env.VITE_API_URL ?? '').replace(/\/+$/, '')
const API_BASE = API_ORIGIN + '/api/v1'

// assetUrl turns a server-relative path like "/uploads/menu/foo.jpg" into a
// full URL when the frontend is served from a different origin than the API
// (i.e. always in production).
export function assetUrl(path: string | undefined | null): string | undefined {
  if (!path) return undefined
  if (/^https?:\/\//.test(path)) return path
  return API_ORIGIN + path
}

const TOKEN_KEY = 'cafe_jwt'
const ADMIN_TOKEN_KEY = 'cafe_admin_jwt'

export type ApiError = { code: string; message: string }

export class ApiCallError extends Error {
  code: string
  status: number
  constructor(code: string, message: string, status: number) {
    super(message)
    this.code = code
    this.status = status
  }
}

function makeStore(key: string) {
  return {
    get(): string | null { return localStorage.getItem(key) },
    set(token: string)    { localStorage.setItem(key, token) },
    clear()                { localStorage.removeItem(key) },
  }
}

export const tokenStore = makeStore(TOKEN_KEY)
export const adminTokenStore = makeStore(ADMIN_TOKEN_KEY)

// `mode` decides which JWT to attach. 'customer' uses tokenStore (default for
// the customer app), 'admin' uses adminTokenStore (admin dashboard), 'public'
// attaches no Authorization header at all.
type Mode = 'customer' | 'admin' | 'public'

async function request<T>(path: string, init: RequestInit = {}, mode: Mode = 'customer'): Promise<T | null> {
  const headers = new Headers(init.headers)
  if (mode !== 'public') {
    const token = (mode === 'admin' ? adminTokenStore : tokenStore).get()
    if (token) headers.set('Authorization', `Bearer ${token}`)
  }
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')

  const res = await fetch(API_BASE + path, { ...init, headers })

  // 204 No Content (e.g. no active order on table) → return null.
  if (res.status === 204) return null

  const text = await res.text()
  const body = text ? safeParse(text) : {}
  if (!res.ok) {
    const err: ApiError = body?.error ?? { code: `http_${res.status}`, message: text || res.statusText }
    throw new ApiCallError(err.code, err.message, res.status)
  }
  return body as T
}

function safeParse(text: string): any {
  try {
    return JSON.parse(text)
  } catch {
    return {}
  }
}

// ----- shared types -----

export type ContextResponse = {
  org: { id: number; name: string }
  table: { id: number; code: string; label?: string }
}

export type Customer = {
  id: number
  email?: string
  mobile_number?: string
  name?: string
}

export type MenuItem = {
  id: number
  name: string
  description?: string
  category?: string
  price: number
  image_url?: string
  display_order: number
}

export type MenuResponse = { items: MenuItem[] }

export type OrderItem = {
  id: number
  order_id: number
  org_id: number
  menu_item_id?: number
  item_name: string
  unit_price: number
  quantity: number
  line_total: number
  added_by_customer_id?: number
  created_at: string
}

export type OrderStatus = 'queued' | 'cooking' | 'prepared' | 'completed' | 'cancelled'

export type Order = {
  id: number
  public_code: string
  org_id: number
  table_id: number
  customer_id?: number
  status: OrderStatus
  total_amount: number
  is_paid: boolean
  placed_at: string
  completed_at?: string
  items: OrderItem[]
}

// ----- endpoints -----

export const api = {
  getContext(orgId: string | number, tableCode: string) {
    const params = new URLSearchParams({ org_id: String(orgId), table_code: tableCode })
    return request<ContextResponse>(`/public/context?${params.toString()}`) as Promise<ContextResponse>
  },

  requestOtp(email: string, mobileNumber?: string) {
    const body: Record<string, string> = { email }
    if (mobileNumber) body.mobile_number = mobileNumber
    return request<{ request_id: number; expires_in: number; dev_otp?: string }>(
      '/auth/otp/request',
      { method: 'POST', body: JSON.stringify(body) },
    ) as Promise<{ request_id: number; expires_in: number; dev_otp?: string }>
  },

  verifyOtp(email: string, code: string) {
    return request<{ jwt: string; customer: Customer }>('/auth/otp/verify', {
      method: 'POST',
      body: JSON.stringify({ email, code }),
    }) as Promise<{ jwt: string; customer: Customer }>
  },

  me() {
    return request<Customer>('/me') as Promise<Customer>
  },

  logout() {
    return request<{ ok: boolean }>('/auth/logout', { method: 'POST' }) as Promise<{ ok: boolean }>
  },

  getMenu(orgId: string | number) {
    return request<MenuResponse>(`/orgs/${orgId}/menu`) as Promise<MenuResponse>
  },

  /** Returns the table's open order, or null if none. */
  getActiveOrder(orgId: string | number, tableCode: string) {
    return request<Order>(`/orgs/${orgId}/tables/${tableCode}/active-order`)
  },

  /**
   * Apply quantity changes to the table's open order. Each item.delta is signed:
   *   delta > 0 → add that many more
   *   delta < 0 → reduce the placed quantity (server deletes/reduces order_items)
   * Creates a new order if none exists (only positive deltas allowed in that case).
   */
  placeOrAppend(input: {
    org_id: number
    table_id: number
    items: { menu_item_id: number; delta: number }[]
  }) {
    return request<Order>('/orders', {
      method: 'POST',
      body: JSON.stringify(input),
    }) as Promise<Order>
  },

  getOrder(publicCode: string) {
    return request<Order>(`/orders/${publicCode}`) as Promise<Order>
  },

  markPaid(publicCode: string) {
    return request<{ ok: boolean }>(`/orders/${publicCode}/mark-paid`, { method: 'POST' }) as Promise<{ ok: boolean }>
  },
}

// ===== Admin / staff side =====

export type StaffRole = 'super_admin' | 'manager' | 'staff'

export type StaffUser = {
  id: number
  org_id?: number
  email?: string
  name?: string
  role: StaffRole
}

export type AdminOrder = Order & {
  table_label?: string
  table_code?: string
  customer_email?: string
}

export type StaffRow = {
  id: number
  org_id?: number
  email?: string
  mobile_number?: string
  name?: string
  role: StaffRole
  is_active: boolean
  created_at: string
}

export type AdminMenuItem = {
  id: number
  org_id: number
  name: string
  description?: string
  category?: string
  price: number
  image_url?: string
  display_order: number
  is_available: boolean
}

export type AdminTable = {
  id: number
  org_id: number
  code: string
  label?: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export type Organisation = {
  id: number
  name: string
  address?: string
  contact_phone?: string
  contact_email?: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export type OrgWithStats = Organisation & {
  staff_count: number
  table_count: number
  menu_count: number
  order_count: number
}

export type StaffWithOrg = StaffRow & { org_name?: string }

export const adminApi = {
  requestOtp(email: string) {
    return request<{ request_id: number; expires_in: number; dev_otp?: string }>(
      '/admin/auth/otp/request',
      { method: 'POST', body: JSON.stringify({ email }) },
      'public',
    ) as Promise<{ request_id: number; expires_in: number; dev_otp?: string }>
  },
  verifyOtp(email: string, code: string) {
    return request<{ jwt: string; staff: StaffUser }>(
      '/admin/auth/otp/verify',
      { method: 'POST', body: JSON.stringify({ email, code }) },
      'public',
    ) as Promise<{ jwt: string; staff: StaffUser }>
  },
  me() {
    return request<StaffUser>('/admin/me', {}, 'admin') as Promise<StaffUser>
  },
  logout() {
    return request<{ ok: boolean }>('/admin/auth/logout', { method: 'POST' }, 'admin') as Promise<{ ok: boolean }>
  },
  listActive() {
    return request<{ orders: AdminOrder[] }>('/admin/orders/active', {}, 'admin') as Promise<{ orders: AdminOrder[] }>
  },
  listHistory(limit = 10, offset = 0) {
    const q = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    return request<{ orders: AdminOrder[]; total: number; limit: number; offset: number }>(
      `/admin/orders/history?${q}`,
      {},
      'admin',
    ) as Promise<{ orders: AdminOrder[]; total: number; limit: number; offset: number }>
  },
  updateStatus(publicCode: string, status: 'cooking' | 'prepared' | 'cancelled') {
    return request<AdminOrder>(
      `/admin/orders/${publicCode}/status`,
      { method: 'PATCH', body: JSON.stringify({ status }) },
      'admin',
    ) as Promise<AdminOrder>
  },
  complete(publicCode: string, body: { method: string; amount: number; txn_ref?: string }) {
    return request<AdminOrder>(
      `/admin/orders/${publicCode}/complete`,
      { method: 'POST', body: JSON.stringify(body) },
      'admin',
    ) as Promise<AdminOrder>
  },

  listStaff() {
    return request<{ staff: StaffRow[] }>('/admin/staff', {}, 'admin') as Promise<{ staff: StaffRow[] }>
  },
  createStaff(body: { email: string; name: string; role: 'manager' | 'staff'; mobile_number?: string }) {
    return request<StaffRow>(
      '/admin/staff',
      { method: 'POST', body: JSON.stringify(body) },
      'admin',
    ) as Promise<StaffRow>
  },
  updateStaff(id: number, body: { name?: string; role?: 'manager' | 'staff'; is_active?: boolean; mobile_number?: string }) {
    return request<StaffRow>(
      `/admin/staff/${id}`,
      { method: 'PATCH', body: JSON.stringify(body) },
      'admin',
    ) as Promise<StaffRow>
  },

  // ----- menu CRUD -----
  listMenu() {
    return request<{ items: AdminMenuItem[] }>('/admin/menu', {}, 'admin') as Promise<{ items: AdminMenuItem[] }>
  },
  createMenu(body: {
    name: string
    description?: string
    category?: string
    price: number
    image_url?: string
    display_order?: number
    is_available?: boolean
  }) {
    return request<AdminMenuItem>('/admin/menu', { method: 'POST', body: JSON.stringify(body) }, 'admin') as Promise<AdminMenuItem>
  },
  updateMenu(id: number, body: Partial<{
    name: string
    description: string | null
    category: string | null
    price: number
    image_url: string | null
    display_order: number
    is_available: boolean
  }>) {
    return request<AdminMenuItem>(`/admin/menu/${id}`, { method: 'PATCH', body: JSON.stringify(body) }, 'admin') as Promise<AdminMenuItem>
  },
  async uploadMenuImage(id: number, file: File): Promise<AdminMenuItem> {
    // multipart upload — bypass the JSON request() helper.
    const fd = new FormData()
    fd.append('file', file)
    const token = adminTokenStore.get()
    const res = await fetch(`${API_BASE}/admin/menu/${id}/image`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    })
    const text = await res.text()
    const body = text ? safeParse(text) : {}
    if (!res.ok) {
      const err = body?.error ?? { code: `http_${res.status}`, message: text || res.statusText }
      throw new ApiCallError(err.code, err.message, res.status)
    }
    return body as AdminMenuItem
  },

  // ----- tables CRUD -----
  listTables() {
    return request<{ tables: AdminTable[] }>('/admin/tables', {}, 'admin') as Promise<{ tables: AdminTable[] }>
  },
  createTable(body: { label?: string }) {
    return request<AdminTable>('/admin/tables', { method: 'POST', body: JSON.stringify(body) }, 'admin') as Promise<AdminTable>
  },
  updateTable(id: number, body: { label?: string; is_active?: boolean }) {
    return request<AdminTable>(`/admin/tables/${id}`, { method: 'PATCH', body: JSON.stringify(body) }, 'admin') as Promise<AdminTable>
  },

  // ----- super_admin (platform-level) -----
  listOrgs() {
    return request<{ orgs: OrgWithStats[] }>('/super/orgs', {}, 'admin') as Promise<{ orgs: OrgWithStats[] }>
  },
  createOrgWithManager(body: {
    org: { name: string; address?: string; contact_phone?: string; contact_email?: string }
    manager: { email: string; name: string; mobile_number?: string }
  }) {
    return request<{ org: Organisation; manager: StaffRow }>(
      '/super/orgs',
      { method: 'POST', body: JSON.stringify(body) },
      'admin',
    ) as Promise<{ org: Organisation; manager: StaffRow }>
  },
  updateOrg(id: number, body: Partial<{ name: string; address: string | null; contact_phone: string | null; contact_email: string | null; is_active: boolean }>) {
    return request<Organisation>(`/super/orgs/${id}`, { method: 'PATCH', body: JSON.stringify(body) }, 'admin') as Promise<Organisation>
  },
  listAllStaff(orgId?: number) {
    const q = orgId ? `?org_id=${orgId}` : ''
    return request<{ staff: StaffWithOrg[] }>(`/super/staff${q}`, {}, 'admin') as Promise<{ staff: StaffWithOrg[] }>
  },
  createStaffInOrg(body: { org_id: number; email: string; name: string; role: 'manager' | 'staff'; mobile_number?: string }) {
    return request<StaffRow>('/super/staff', { method: 'POST', body: JSON.stringify(body) }, 'admin') as Promise<StaffRow>
  },
  updateAnyStaff(id: number, body: { name?: string; role?: 'manager' | 'staff'; is_active?: boolean; mobile_number?: string }) {
    return request<StaffRow>(`/super/staff/${id}`, { method: 'PATCH', body: JSON.stringify(body) }, 'admin') as Promise<StaffRow>
  },
}
