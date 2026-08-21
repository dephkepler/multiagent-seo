const KEY = 'mas_token'
const ROLE_KEY = 'mas_role'

export type Role = 'admin' | 'advocate'

// A session lives in one of two stores. localStorage is shared by every tab in
// the browser, which is what you want day to day — and exactly what makes it
// impossible to hold an admin and an advocate session side by side, because the
// second login overwrites the first. sessionStorage is per tab, so a tab that
// logs in "only here" keeps its own role while the other tabs keep theirs.
//
// Resolution order matters: a tab-scoped session wins, otherwise the
// browser-wide one is used. Token and role are always read from the same store,
// or a tab would end up with one role's token and the other's menu.
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

// setSession stores what the login returned. The role decides nothing on its
// own: every request is authorised from the user row on the server, so editing
// this value in devtools changes the menu and nothing else.
export function setSession(token: string, role: string, tabOnly: boolean) {
  if (typeof window === 'undefined') return

  if (tabOnly) {
    // localStorage is deliberately left alone — that is another tab's session.
    window.sessionStorage.setItem(KEY, token)
    window.sessionStorage.setItem(ROLE_KEY, role)
    return
  }

  // A browser-wide login must not be shadowed by a leftover tab session in
  // this very tab, so that one goes first.
  window.sessionStorage.removeItem(KEY)
  window.sessionStorage.removeItem(ROLE_KEY)
  window.localStorage.setItem(KEY, token)
  window.localStorage.setItem(ROLE_KEY, role)
}

// clearToken signs out of the session this tab is actually using. Signing out
// of a tab-scoped advocate session must not log the admin out of every other
// tab, which is the whole point of having two.
export function clearToken() {
  if (typeof window === 'undefined') return

  const store = activeStore()
  if (!store) {
    window.sessionStorage.removeItem(ROLE_KEY)
    window.localStorage.removeItem(ROLE_KEY)
    return
  }
  store.removeItem(KEY)
  store.removeItem(ROLE_KEY)
}
