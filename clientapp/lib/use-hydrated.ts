'use client'

import { useSyncExternalStore } from 'react'

const noopSubscribe = () => () => {}

/**
 * False while rendering on the server and during hydration, true afterwards.
 *
 * The app is a static export, so its HTML is rendered at build time where there
 * is no `window` and therefore no Telegram. Anything that branches on Telegram
 * being present has to wait for hydration or the markup will not match.
 *
 * Written with useSyncExternalStore rather than the usual
 * `useState(false)` + `setState(true)` in an effect: that pattern sets state
 * synchronously inside an effect body, which React flags as a cascading render.
 * This is the primitive built for exactly this question.
 */
export function useHydrated(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => true,
    () => false
  )
}
