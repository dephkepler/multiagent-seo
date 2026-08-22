const KEY = 'mas_token'
const ROLE_KEY = 'mas_role'

export type Role = 'admin' | 'advocate'

// A session is kept in two places on purpose.
//
// sessionStorage belongs to one tab, so a tab that has a session keeps it no
// matter who signs in elsewhere — that is what stops a second account in
// another tab from evicting the first and bouncing you out of the page you were
// reading. localStorage holds the most recent login, so a newly opened tab is
// not asked to sign in again.
//
// Reads take the tab's own copy first. Token and role always come from the same
// store, or a tab would end up holding one account's token and the other's menu.
function activeStore(): Storage | null {
  if (typeof window === 'undefined') return null
  if (window.sessionStorage.getItem(KEY)) return window.sessionStorage
  if (window.localStorage.getItem(KEY)) return window.localStorage
  return null
}

export function getToken(): string | null {
  return activeStore()?.getItem(KEY) ?? null
}

export function getRole(): Role {
  return activeStore()?.getItem(ROLE_KEY) === 'advocate' ? 'advocate' : 'admin'
}

// pinSession copies the browser-wide session into this tab, once. Called when
// the app shell mounts: from then on the tab is fixed to the account it opened
// with, and a sign-in in a different tab cannot change it under you.
export function pinSession() {
  if (typeof window === 'undefined') return
  if (window.sessionStorage.getItem(KEY)) return

  const token = window.localStorage.getItem(KEY)
  if (!token) return
  window.sessionStorage.setItem(KEY, token)
  window.sessionStorage.setItem(ROLE_KEY, window.localStorage.getItem(ROLE_KEY) ?? 'admin')
}

// setSession stores what the login returned: in this tab, and as the browser's
// most recent login. The role decides nothing on its own — every request is
// authorised from the user row on the server, so editing this value in devtools
// changes the menu and nothing else.
export function setSession(token: string, role: string) {
  if (typeof window === 'undefined') return

  window.sessionStorage.setItem(KEY, token)
  window.sessionStorage.setItem(ROLE_KEY, role)
  window.localStorage.setItem(KEY, token)
  window.localStorage.setItem(ROLE_KEY, role)
}

// clearToken signs out of this tab and stops a new tab from reopening the same
// session. Other tabs keep their own — signing out of one account is not a
// reason to throw you out of the other.
export function clearToken() {
  if (typeof window === 'undefined') return

  window.sessionStorage.removeItem(KEY)
  window.sessionStorage.removeItem(ROLE_KEY)
  window.localStorage.removeItem(KEY)
  window.localStorage.removeItem(ROLE_KEY)
}
