'use client'

import { useSyncExternalStore } from 'react'
import { getRole, getToken, type Role } from './auth'

// The role lives in localStorage, which the server render cannot see. Reading it
// through useSyncExternalStore instead of an effect keeps the server snapshot
// ('admin') and the client snapshot explicit, and picks up a sign-in or sign-out
// that happened in another tab.
export function useRole(): Role {
  return useSyncExternalStore(subscribe, getRole, serverRole)
}

// useHasSession answers "is somebody already logged in in this browser". The
// sign-in form uses it to default the second login to this-tab-only, so signing
// in as an advocate cannot silently take over the admin's tabs.
export function useHasSession(): boolean {
  return useSyncExternalStore(subscribe, hasSession, serverHasSession)
}

function hasSession(): boolean {
  return getToken() !== null
}

function serverHasSession(): boolean {
  return false
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange)
  return () => window.removeEventListener('storage', onChange)
}

function serverRole(): Role {
  return 'admin'
}
