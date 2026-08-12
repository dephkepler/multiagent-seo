'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { clearToken, getToken } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { cx } from '@/lib/cx'

const links = [
  { href: '/generate', label: 'Generate' },
  { href: '/sites', label: 'Sites' },
  { href: '/linkbuilding/place-backlinks', label: 'Place backlinks' },
  { href: '/emailscrape', label: 'Email scrape' },
  { href: '/leads', label: 'Leads' },
]

export function Nav() {
  const pathname = usePathname()
  const router = useRouter()
  const hasToken = typeof window !== 'undefined' && !!getToken()

  return (
    <nav className='flex h-14 items-center gap-6 border-b border-gray-200 bg-white px-6'>
      <div className='font-semibold tracking-tight'>multiagent-seo</div>
      <div className='flex gap-1'>
        {links.map((l) => {
          const active = pathname === l.href || pathname.startsWith(l.href + '/')
          return (
            <Link
              key={l.href}
              href={l.href}
              className={cx('rounded px-3 py-1.5 text-sm', active ? 'bg-emerald-100 text-emerald-800' : 'text-gray-700 hover:bg-gray-100')}
            >
              {l.label}
            </Link>
          )
        })}
      </div>
      <div className='ml-auto'>
        {hasToken && (
          <Button
            variant='secondary'
            size='sm'
            onClick={() => {
              clearToken()
              router.push('/signin')
            }}
          >
            Sign out
          </Button>
        )}
      </div>
    </nav>
  )
}
