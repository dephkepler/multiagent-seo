'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { getToken } from '@/lib/auth'
import { Nav } from '@/components/layout/nav'

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const [ready, setReady] = useState(false)

  useEffect(() => {
    if (!getToken()) {
      router.replace('/signin')
      return
    }
    setReady(true)
  }, [router])

  if (!ready) return null

  return (
    <div className='min-h-screen'>
      <Nav />
      <main className='mx-auto max-w-7xl p-6'>{children}</main>
    </div>
  )
}
