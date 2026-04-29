async function request(url, options = {}) {
  const r = await fetch(url, { cache: 'no-store', credentials: 'same-origin', ...options })
  if (r.status === 401 && !url.includes('/api/login') && location.hash !== '#/login') {
    location.hash = '#/login'
  }
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
  login: (data) => post('api/login', data),
  logout: () => post('api/logout'),
  getSession: () => request('api/session'),
  getStatus: () => request('api/status'),
  getDaily: () => request('api/daily'),
  getSettings: () => request('api/settings'),
  saveSettings: (data) => post('api/settings', data),
  testEmail: () => post('api/settings/test-email'),
  vacuum: () => post('api/db/vacuum'),
  purge: (days) => post('api/db/purge', { days }),
  exportCSVUrl: 'api/daily/export'
}
