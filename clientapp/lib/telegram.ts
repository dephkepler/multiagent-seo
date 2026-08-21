'use client'

// The Telegram bridge. Everything this app knows about living inside Telegram is
// here; the rest of the code sees an init-data string, a data-theme attribute
// and three CSS variables.

interface SafeAreaInset {
  top: number
  bottom: number
  left: number
  right: number
}

interface TgWebApp {
  initData: string
  version: string
  platform: string
  colorScheme: 'light' | 'dark'
  viewportStableHeight: number
  safeAreaInset?: SafeAreaInset
  contentSafeAreaInset?: SafeAreaInset
  isVersionAtLeast(v: string): boolean
  ready(): void
  expand(): void
  close(): void
  disableVerticalSwipes?(): void
  setHeaderColor?(color: string): void
  setBackgroundColor?(color: string): void
  onEvent(event: string, cb: () => void): void
  offEvent(event: string, cb: () => void): void
  HapticFeedback?: {
    notificationOccurred(type: 'error' | 'success' | 'warning'): void
    impactOccurred(style: 'light' | 'medium' | 'heavy'): void
  }
}

declare global {
  interface Window {
    Telegram?: { WebApp: TgWebApp }
  }
}

function webApp(): TgWebApp | null {
  if (typeof window === 'undefined') return null
  return window.Telegram?.WebApp ?? null
}

// Captured once per launch: init data is a signed snapshot Telegram hands over
// when the app opens and never refreshes, so re-reading it gains nothing — and
// caching means lib/api.ts never touches window at all.
let cached: string | null = null

export function getInitData(): string | null {
  if (cached !== null) return cached
  const wa = webApp()
  if (wa?.initData) {
    cached = wa.initData
    return cached
  }
  // Developing in a plain browser, where nothing can sign anything. Worth
  // nothing on its own: the Go server only accepts it when it was started with
  // CF_TELEGRAM_DEV_USER_ID, which it refuses to do off a developer machine.
  const dev = process.env.NEXT_PUBLIC_DEV_INIT_DATA
  if (dev) {
    cached = dev
    return cached
  }
  return null
}

export function isInsideTelegram(): boolean {
  return getInitData() !== null
}

// We own every colour; Telegram only decides which of our two themes to show,
// and we push our own plane into its chrome so the seam under the header
// disappears. Deriving colours from themeParams was rejected: a user can
// install a Telegram theme of arbitrary colours, and then nothing here can be
// checked for contrast.
const PLANE = { light: '#f4f5f7', dark: '#17181b' } as const

function applyTheme(wa: TgWebApp) {
  const scheme = wa.colorScheme === 'dark' ? 'dark' : 'light'
  document.documentElement.dataset.theme = scheme
  wa.setHeaderColor?.(PLANE[scheme])
  wa.setBackgroundColor?.(PLANE[scheme])
}

function applyViewport(wa: TgWebApp) {
  const root = document.documentElement
  // Stable height, not viewportHeight: the latter tracks the drag mid-gesture,
  // so laying out against it makes the whole page twitch while scrolling.
  root.style.setProperty('--tg-vh', `${wa.viewportStableHeight}px`)
  root.style.setProperty('--tg-safe-top', `${wa.contentSafeAreaInset?.top ?? 0}px`)
  root.style.setProperty('--tg-safe-bottom', `${wa.safeAreaInset?.bottom ?? 0}px`)
}

/** Called once, from the Telegram provider. Returns a cleanup. */
export function initTelegram(): () => void {
  const wa = webApp()
  if (!wa) {
    // Outside Telegram only the "open me in Telegram" screen renders, so at
    // least follow the OS preference rather than blinding anyone.
    const dark = window.matchMedia?.('(prefers-color-scheme: dark)').matches
    document.documentElement.dataset.theme = dark ? 'dark' : 'light'
    return () => {}
  }

  wa.ready()
  wa.expand()
  // 7.7+. Without this an upward scroll on a long form is taken for
  // swipe-to-close and the app shuts mid-typing on Android.
  if (wa.isVersionAtLeast('7.7')) wa.disableVerticalSwipes?.()

  applyTheme(wa)
  applyViewport(wa)

  const onTheme = () => applyTheme(wa)
  const onViewport = () => applyViewport(wa)
  const events: Array<[string, () => void]> = [
    ['themeChanged', onTheme],
    ['viewportChanged', onViewport],
    ['safeAreaChanged', onViewport],
    ['contentSafeAreaChanged', onViewport],
  ]
  events.forEach(([e, cb]) => wa.onEvent(e, cb))
  return () => events.forEach(([e, cb]) => wa.offEvent(e, cb))
}

export function closeApp() {
  webApp()?.close()
}

export function notify(kind: 'success' | 'error' | 'warning') {
  webApp()?.HapticFeedback?.notificationOccurred(kind)
}
