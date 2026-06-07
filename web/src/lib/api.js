// Thin fetch wrapper around weft's JSON API. The CSRF token (returned by
// /login and /me) is echoed back in the X-CSRF-Token header on writes.

let csrf = ''

export function setCsrf(token) {
  csrf = token || ''
}

async function request(method, path, body) {
  const opts = { method, headers: {}, credentials: 'same-origin' }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  if (method !== 'GET') opts.headers['X-CSRF-Token'] = csrf

  const res = await fetch('/api' + path, opts)
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    const err = new Error((data && data.error) || res.statusText)
    err.status = res.status
    throw err
  }
  return data
}

export const api = {
  get: (p) => request('GET', p),
  post: (p, b) => request('POST', p, b),
  put: (p, b) => request('PUT', p, b),
  del: (p) => request('DELETE', p),
}
