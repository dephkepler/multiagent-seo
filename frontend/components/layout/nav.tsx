'use client'

import Image from 'next/image'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { useEffect, useState } from 'react'
import { clearToken, getToken } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { cx } from '@/lib/cx'

const links = [
  { href: '/generate', label: 'Generate' },
  { href: '/sites', label: 'Sites' },
  { href: '/linkbuilding/place-backlinks', label: 'Place backlinks' },
  { href: '/emailscrape', label: 'Email scrape' },
  { href: '/leads', label: 'Leads' },
  { href: '/clients', label: 'Clients' },
]

export function Nav() {
  const pathname = usePathname()
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const hasToken = typeof window !== 'undefined' && !!getToken()

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  return (
    <>
      <nav className='relative z-[60] flex h-14 items-center gap-2 border-b border-gray-200 bg-white px-6'>
        <button
          type='button'
          aria-label={open ? 'Закрыть меню' : 'Открыть меню'}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className={cx('shrink-0 rounded-md transition-transform duration-300 ease-out hover:opacity-80', open && 'rotate-180')}
        >
          <Image src='/icon.png' alt='' width={28} height={28} className='rounded-md' />
        </button>
        <span className='font-semibold tracking-tight'>multiagent-seo</span>
      </nav>

      {/* backdrop — click to close, fades with the drawer */}
      <div
        onClick={() => setOpen(false)}
        aria-hidden
        className={cx(
          'fixed inset-0 z-40 bg-black/30 transition-opacity duration-300',
          open ? 'opacity-100' : 'pointer-events-none opacity-0'
        )}
      />

      {/* left-side menu drawer */}
      <aside
        className={cx(
          'fixed top-0 left-0 z-50 flex h-full w-72 flex-col gap-1 border-r border-gray-200 bg-white p-4 shadow-xl transition-transform duration-300 ease-out',
          open ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        <div className='mb-3 flex items-center gap-2 px-2 pt-1'>
          <Image src='/icon.png' alt='' width={28} height={28} className='rounded-md' />
          <span className='font-semibold'>Меню</span>
        </div>

        <div className='flex flex-1 flex-col gap-1'>
          {links.map((l) => {
            const active = pathname === l.href || pathname.startsWith(l.href + '/')
            return (
              <Link
                key={l.href}
                href={l.href}
                onClick={() => setOpen(false)}
                className={cx(
                  'rounded-md px-3 py-2 text-sm',
                  active ? 'bg-emerald-100 font-medium text-emerald-800' : 'text-gray-700 hover:bg-gray-100'
                )}
              >
                {l.label}
              </Link>
            )
          })}
        </div>

        {hasToken && (
          <Button
            variant='secondary'
            size='sm'
            className='w-full'
            onClick={() => {
              clearToken()
              router.push('/signin')
            }}
          >
            Sign out
          </Button>
        )}
      </aside>
    </>
  )
}
