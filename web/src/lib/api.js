// Thin fetch wrapper around weft's JSON API. The CSRF token (returned by
// /login and /me) is echoed back in the X-CSRF-Token header on writes.
//
// Session expiry: the server expires a session after session_timeout of API
// inactivity (sliding). We mirror that on the client with a timer that is reset
// on every successful request, so an idle browser switches to the login view at
// roughly the same moment — and a 401 (session already gone) does so immediately.

let csrf = ''
let sessionMs = 0
let expireTimer = null
let onSessionLost = null

export function setCsrf(token) {
  csrf = token || ''
}

// configureSession enables the inactivity auto-logout: after `seconds` without a
// successful request, onLost() is called (clear state -> login view).
export function configureSession(seconds, onLost) {
  sessionMs = (seconds || 0) * 1000
  onSessionLost = onLost
  armTimer()
}

// endSession disables the timer (called on explicit logout).
export function endSession() {
  if (expireTimer) clearTimeout(expireTimer)
  expireTimer = null
  onSessionLost = null
}

function armTimer() {
  if (expireTimer) clearTimeout(expireTimer)
  if (!sessionMs || !onSessionLost) return
  expireTimer = setTimeout(() => {
    expireTimer = null
    const cb = onSessionLost
    onSessionLost = null
    cb && cb()
  }, sessionMs)
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

  if (res.status === 401 && path !== '/login' && onSessionLost) {
    const cb = onSessionLost
    onSessionLost = null
    if (expireTimer) clearTimeout(expireTimer)
    expireTimer = null
    cb()
  } else if (res.ok && path !== '/logout') {
    armTimer() // a successful request keeps the session alive; reset the timer
  }

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
