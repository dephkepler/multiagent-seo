'use client'

import Image from 'next/image'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { useEffect, useState } from 'react'
import { clearToken, getToken } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { cx } from '@/lib/cx'

const groups = [
  {
    label: 'Контент',
    links: [
      { href: '/generate', label: 'Generate' },
      { href: '/sites', label: 'Sites' },
    ],
  },
  {
    label: 'Линкбилдинг',
    links: [
      { href: '/linkbuilding/place-backlinks', label: 'Place backlinks' },
      { href: '/emailscrape', label: 'Email scrape' },
    ],
  },
  {
    label: 'CRM',
    links: [
      { href: '/leads', label: 'Leads' },
      { href: '/clients', label: 'Clients' },
    ],
  },
  {
    label: 'Админ',
    links: [
      { href: '/finance', label: 'Finance' },
      { href: '/vault', label: 'Vault' },
    ],
  },
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
      <nav className='relative z-[60] flex h-14 items-center gap-2 border-b border-gray-200 bg-white px-4'>
        <button
          type='button'
          aria-label={open ? 'Закрыть меню' : 'Открыть меню'}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className='shrink-0 rounded-md p-1 transition-colors hover:bg-gray-100'
        >
          <Image
            src='/icon.png'
            alt=''
            width={28}
            height={28}
            className={cx('rounded-md transition-transform duration-300 ease-out', open && 'rotate-180')}
          />
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
          'fixed top-0 left-0 z-50 flex h-full w-72 flex-col border-r border-gray-200 bg-white shadow-xl transition-transform duration-300 ease-out',
          open ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        <div className='flex items-center gap-2 border-b border-gray-100 px-4 py-3.5'>
          <Image src='/icon.png' alt='' width={28} height={28} className='rounded-md' />
          <span className='font-semibold'>Меню</span>
        </div>

        <nav className='flex-1 space-y-4 overflow-y-auto px-3 py-4'>
          {groups.map((group) => (
            <div key={group.label}>
              <div className='mb-1 px-3 text-[11px] font-semibold tracking-wide text-gray-400 uppercase'>{group.label}</div>
              <div className='flex flex-col gap-0.5'>
                {group.links.map((l) => {
                  const active = pathname === l.href || pathname.startsWith(l.href + '/')
                  return (
                    <Link
                      key={l.href}
                      href={l.href}
                      onClick={() => setOpen(false)}
                      className={cx(
                        'rounded-md px-3 py-2 text-sm transition-colors',
                        active ? 'bg-emerald-100 font-medium text-emerald-800' : 'text-gray-700 hover:bg-gray-100'
                      )}
                    >
                      {l.label}
                    </Link>
                  )
                })}
              </div>
            </div>
          ))}
        </nav>

        {hasToken && (
          <div className='border-t border-gray-100 p-3'>
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
          </div>
        )}
      </aside>
    </>
  )
}
