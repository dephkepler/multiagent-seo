const KEY = 'mas_token'
const ROLE_KEY = 'mas_role'

export type Role = 'admin' | 'advocate'

export function getToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem(KEY)
}

export function setToken(token: string) {
  if (typeof window === 'undefined') return
  localStorage.setItem(KEY, token)
}

// The role is kept next to the token so the UI knows which section to open
// without a round trip. It decides nothing: every request is authorised from
// the user row on the server, so editing this value in devtools changes what
// the menu looks like and nothing else.
export function setRole(role: string) {
  if (typeof window === 'undefined') return
  localStorage.setItem(ROLE_KEY, role)
}

export function getRole(): Role {
  if (typeof window === 'undefined') return 'admin'
  return localStorage.getItem(ROLE_KEY) === 'advocate' ? 'advocate' : 'admin'
}

export function clearToken() {
  if (typeof window === 'undefined') return
  localStorage.removeItem(KEY)
  localStorage.removeItem(ROLE_KEY)
}
