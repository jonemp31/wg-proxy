const API_BASE = '/api'

function getToken() {
  return localStorage.getItem('token')
}

export function setToken(token) {
  localStorage.setItem('token', token)
}

export function clearToken() {
  localStorage.removeItem('token')
}

export function hasToken() {
  return !!localStorage.getItem('token')
}

async function request(path, options = {}) {
  const token = getToken()
  const headers = { 'Content-Type': 'application/json' }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const resp = await fetch(`${API_BASE}${path}`, {
    headers,
    ...options,
    headers: { ...headers, ...(options.headers || {}) },
  })

  if (resp.status === 401) {
    clearToken()
    window.dispatchEvent(new Event('auth:logout'))
    throw new Error('Sessão expirada')
  }

  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw new Error(err.error || 'Request failed')
  }
  return resp
}

export async function login(username, password) {
  const resp = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const data = await resp.json()
  if (!resp.ok) {
    const error = new Error(data.error || 'Login failed')
    error.status = resp.status
    error.retryAfter = data.retry_after_s
    throw error
  }
  setToken(data.token)
  return data
}

export async function changePassword(currentPassword, newPassword) {
  const resp = await request('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
  return resp.json()
}

export async function getMe() {
  const resp = await request('/auth/me')
  return resp.json()
}

export async function getDevices() {
  const resp = await request('/devices')
  return resp.json()
}

export async function createDevice(name) {
  const resp = await request('/devices', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
  return resp.json()
}

export async function getDevice(id) {
  const resp = await request(`/devices/${id}`)
  return resp.json()
}

export async function deleteDevice(id) {
  await request(`/devices/${id}`, { method: 'DELETE' })
}

export async function getQRCodeUrl(id) {
  const token = getToken()
  return `${API_BASE}/devices/${id}/qrcode?token=${encodeURIComponent(token)}`
}

export async function getProxies() {
  const resp = await request('/proxies')
  return resp.json()
}

export async function getMetrics() {
  const resp = await request('/metrics')
  return resp.json()
}

export async function getDeviceMetrics(id, period = '24h') {
  const resp = await request(`/metrics/${id}?period=${period}`)
  return resp.json()
}

export async function getWebhook() {
  const resp = await request('/settings/webhook')
  return resp.json()
}

export async function setWebhook(url) {
  const resp = await request('/settings/webhook', {
    method: 'PUT',
    body: JSON.stringify({ url }),
  })
  return resp.json()
}

export async function deleteWebhook() {
  const resp = await request('/settings/webhook', { method: 'DELETE' })
  return resp.json()
}

export async function testWebhook() {
  const resp = await request('/settings/webhook/test', { method: 'POST' })
  return resp.json()
}
