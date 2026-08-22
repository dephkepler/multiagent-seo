'use client'

import { useEffect, useState } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { getRole, getToken, pinSession } from '@/lib/auth'
import { Nav } from '@/components/layout/nav'

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const pathname = usePathname()
  const [ready, setReady] = useState(false)

  useEffect(() => {
    // Fix this tab to the account it opened with, before anything reads the
    // role: otherwise signing in as somebody else in another tab would move
    // this one too, which is exactly the "it keeps throwing me out" complaint.
    pinSession()

    if (!getToken()) {
      router.replace('/signin')
      return
    }
    // An advocate typing an admin URL is sent to their own section instead of
    // watching a page load and then fail with 403s. This is convenience, not
    // protection: the API refuses those requests regardless of what the
    // browser thinks its role is.
    if (getRole() === 'advocate' && !pathname.startsWith('/my')) {
      router.replace('/my')
      return
    }
    setReady(true)
  }, [router, pathname])

  if (!ready) return <div className='min-h-screen' aria-busy='true' />

  return (
    <div className='min-h-screen'>
      <Nav />
      <main className='mx-auto max-w-7xl p-4 sm:p-6'>{children}</main>
    </div>
  )
}
