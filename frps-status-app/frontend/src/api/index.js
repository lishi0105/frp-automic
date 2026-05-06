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

function withQuery(url, params = {}) {
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    qs.set(k, String(v))
  }
  const s = qs.toString()
  return s ? `${url}?${s}` : url
}

export const api = {
  login: (data) => post('api/login', data),
  logout: () => post('api/logout'),
  getSession: () => request('api/session'),
  getStatus: () => request('api/status'),
  getHostNetwork: () => request('api/host-network'),
  getProxies: (params) => request(withQuery('api/proxies', params)),
  getCertificates: (params) => request(withQuery('api/certificates', params)),
  getDaily: () => request('api/daily'),
  getDailyInterface: (params) => request(withQuery('api/daily/interface', params)),
  getSettings: () => request('api/settings'),
  saveSettings: (data) => post('api/settings', data),
  testEmail: () => post('api/settings/test-email'),
  vacuum: () => post('api/db/vacuum'),
  purge: (days) => post('api/db/purge', { days }),
  exportCSVUrl: 'api/daily/export',
  getCurrentLog: () => request('api/logs/current'),
  clearCurrentLog: () => post('api/logs/clear'),
  getWarnings: () => request('api/warnings'),
  getUser: () => request('api/user'),
  changeCredentials: (data) => post('api/user/credentials', data),
  changeRecoveryEmail: (data) => post('api/user/recovery-email', data),
  forgotPassword: (data) => post('api/user/forgot-password', data)
}
