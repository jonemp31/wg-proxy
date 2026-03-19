const API_BASE = '/api'

async function request(path, options = {}) {
  const resp = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw new Error(err.error || 'Request failed')
  }
  return resp
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
  return `${API_BASE}/devices/${id}/qrcode`
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
