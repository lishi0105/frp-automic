async function request(url, options = {}) {
  const r = await fetch(url, { cache: 'no-store', ...options })
  if (!r.ok) throw new Error((await r.text()) || r.statusText)
  const ct = r.headers.get('content-type') || ''
  return ct.includes('json') ? r.json() : r.text()
}

function post(url, body) {
  return request(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body != null ? JSON.stringify(body) : undefined
  })
}

export const api = {
  getStatus: () => request('api/status'),
  getDaily: () => request('api/daily'),
  getSettings: () => request('api/settings'),
  saveSettings: (data) => post('api/settings', data),
  testEmail: () => post('api/settings/test-email'),
  vacuum: () => post('api/db/vacuum'),
  purge: (days) => post('api/db/purge', { days }),
  exportCSVUrl: 'api/daily/export'
}
