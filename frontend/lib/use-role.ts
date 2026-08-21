'use client'

import { useSyncExternalStore } from 'react'
import { getRole, type Role } from './auth'

// The role lives in localStorage, which the server render cannot see. Reading it
// through useSyncExternalStore instead of an effect keeps the server snapshot
// ('admin') and the client snapshot explicit, and picks up a sign-in or sign-out
// that happened in another tab.
export function useRole(): Role {
  return useSyncExternalStore(subscribe, getRole, serverRole)
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange)
  return () => window.removeEventListener('storage', onChange)
}

function serverRole(): Role {
  return 'admin'
}
