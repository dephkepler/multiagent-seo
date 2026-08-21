'use client'

import { useQuery } from '@tanstack/react-query'
import { api, NotAClientError, StaleLaunchError } from '@/lib/api'
import type { Profile } from '@/lib/api-types'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { closeApp } from '@/lib/telegram'
import { Appointments } from './appointments'
import { BookingForm } from './book/booking-form'

export default function Page() {
  // A 403 here is the expected answer for someone the CRM has never seen: they
  // are a guest, and the form below is what turns them into a client. So this
  // query decides how much of the page to show, not whether the page works.
  const profile = useQuery({
    queryKey: ['profile'],
    queryFn: () => api<Profile>('/client/me'),
  })

  if (profile.isError && profile.error instanceof StaleLaunchError) {
    return (
      <Shell>
        <Card role='alert' className='text-center'>
          <p className='text-body text-ink-2'>Цей сеанс більше не діє. Закрийте застосунок і відкрийте знову.</p>
          <Button className='mt-3' onClick={closeApp}>
            Закрити
          </Button>
        </Card>
      </Shell>
    )
  }

  if (profile.isPending) {
    return (
      <Shell>
        <Skeleton className='h-28' />
        <Skeleton className='h-64' />
      </Shell>
    )
  }

  const known = profile.data && !(profile.error instanceof NotAClientError)

  return (
    <Shell>
      <header className='px-1'>
        <h1 className='text-h1 font-semibold text-ink'>
          {known ? `Вітаємо, ${firstName(profile.data.name)}` : 'Консультація адвоката'}
        </h1>
        <p className='mt-1 text-body text-ink-2'>
          {known ? 'Тут видно Ваші записи та можна записатися ще.' : 'Заповніть заявку — і виберіть зручну годину.'}
        </p>
      </header>

      {known && (
        <Appointments consultations={profile.data.consultations} notificationsOn={profile.data.notifications_on} />
      )}

      <BookingForm defaults={known ? { name: profile.data.name, phone: profile.data.phone } : undefined} />
    </Shell>
  )
}

function Shell({ children }: { children: React.ReactNode }) {
  return <main className='tg-shell mx-auto flex max-w-[560px] flex-col gap-4 px-4 pt-4'>{children}</main>
}

// The full ПІБ in a greeting reads like a summons, which is the opposite of the
// tone wanted on a law firm's booking screen.
function firstName(full: string): string {
  const parts = full.trim().split(/\s+/)
  return parts.length > 1 ? parts[1] : parts[0] || ''
}
