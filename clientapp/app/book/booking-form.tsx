'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, SlotTakenError } from '@/lib/api'
import type { BookingOptions, RequestBody, RequestResult } from '@/lib/api-types'
import { Card } from '@/components/ui/card'
import { Button, Chip } from '@/components/ui/button'
import { Field, TextArea, TextInput } from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime } from '@/lib/format'
import { notify } from '@/lib/telegram'
import { SlotPicker } from './slot-picker'

export function BookingForm({ defaults }: { defaults?: { name?: string; phone?: string } }) {
  const queryClient = useQueryClient()
  const options = useQuery({
    queryKey: ['booking-options'],
    queryFn: () => api<BookingOptions>('/client/booking-options'),
  })

  const [name, setName] = useState(defaults?.name ?? '')
  const [phone, setPhone] = useState(defaults?.phone ?? '')
  const [email, setEmail] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const [question, setQuestion] = useState('')
  const [slot, setSlot] = useState<string | null>(null)
  const [touched, setTouched] = useState(false)

  const submit = useMutation({
    mutationFn: (body: RequestBody) =>
      api<RequestResult>('/client/requests', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      notify('success')
      // The profile now has a consultation on it, and the grid lost an hour.
      queryClient.invalidateQueries({ queryKey: ['profile'] })
      queryClient.invalidateQueries({ queryKey: ['booking-options'] })
    },
    onError: (error) => {
      notify('error')
      // Somebody took the hour first: drop the selection and redraw, so the
      // client is not staring at a button that will keep failing.
      if (error instanceof SlotTakenError) {
        setSlot(null)
        queryClient.invalidateQueries({ queryKey: ['booking-options'] })
      }
    },
  })

  if (submit.isSuccess) {
    return <Submitted at={submit.data.consultation?.scheduled_at} onAgain={() => submit.reset()} />
  }

  const nameError = touched && name.trim() === '' ? 'Вкажіть, будь ласка, ПІБ' : undefined
  const phoneError = touched && phone.trim() === '' ? 'Вкажіть номер телефону' : undefined

  function onSubmit(event: React.FormEvent) {
    event.preventDefault()
    setTouched(true)
    if (name.trim() === '' || phone.trim() === '') return
    submit.mutate({
      name: name.trim(),
      phone: phone.trim(),
      email: email.trim() || undefined,
      category: category ?? undefined,
      question: question.trim() || undefined,
      slot: slot ?? undefined,
    })
  }

  return (
    <form onSubmit={onSubmit} className='flex flex-col gap-4' noValidate>
      <Card className='flex flex-col gap-4'>
        <Field label='ПІБ' error={nameError}>
          {({ id, describedBy }) => (
            <TextInput
              id={id}
              aria-describedby={describedBy}
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoComplete='name'
              placeholder='Коваль Петро Іванович'
            />
          )}
        </Field>

        <Field label='Телефон' hint='Щоб адвокат міг зв’язатися' error={phoneError}>
          {({ id, describedBy }) => (
            <TextInput
              id={id}
              aria-describedby={describedBy}
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              type='tel'
              inputMode='tel'
              autoComplete='tel'
              placeholder='+380 50 111 22 33'
            />
          )}
        </Field>

        <Field label='Email' hint='Необов’язково'>
          {({ id, describedBy }) => (
            <TextInput
              id={id}
              aria-describedby={describedBy}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              type='email'
              inputMode='email'
              autoComplete='email'
            />
          )}
        </Field>
      </Card>

      <Card className='flex flex-col gap-3'>
        <h2 className='text-h2 font-semibold text-ink'>Яка допомога потрібна?</h2>
        {options.isPending ? (
          <Skeleton className='h-24' />
        ) : (
          <div className='flex flex-wrap gap-2'>
            {(options.data?.categories ?? []).map((item) => (
              <Chip key={item} selected={item === category} onClick={() => setCategory(item === category ? null : item)}>
                {item}
              </Chip>
            ))}
          </div>
        )}

        <Field label='Коротко про питання'>
          {({ id }) => (
            <TextArea id={id} value={question} onChange={(e) => setQuestion(e.target.value)} placeholder='' />
          )}
        </Field>
      </Card>

      <Card className='flex flex-col gap-3'>
        <div>
          <h2 className='text-h2 font-semibold text-ink'>Час консультації</h2>
          <p className='mt-1 text-small text-ink-3'>Необов’язково — можемо просто зателефонувати.</p>
        </div>
        {options.isPending ? (
          <Skeleton className='h-32' />
        ) : options.isError ? (
          <p className='text-body text-ink-2'>Не вдалося завантажити вільні години. Заявку можна надіслати без часу.</p>
        ) : (
          <SlotPicker slots={options.data?.slots ?? []} selected={slot} onSelect={setSlot} />
        )}
      </Card>

      {submit.isError && (
        <Card role='alert'>
          <p className='text-body text-ink-2'>
            {submit.error instanceof SlotTakenError
              ? 'Цю годину щойно зайняли. Виберіть, будь ласка, іншу.'
              : 'Не вдалося надіслати заявку. Спробуйте ще раз.'}
          </p>
        </Card>
      )}

      <Button type='submit' disabled={submit.isPending}>
        {submit.isPending ? 'Надсилаємо…' : slot ? 'Записатися' : 'Надіслати заявку'}
      </Button>
    </form>
  )
}

function Submitted({ at, onAgain }: { at?: string; onAgain: () => void }) {
  return (
    <Card className='flex flex-col gap-3 text-center'>
      <h2 className='text-h1 font-semibold text-ink'>Заявку прийнято</h2>
      {at ? (
        <p className='text-body text-ink-2'>
          Ми зарезервували {formatDateTime(at)}. Адвокат підтвердить запис — сповіщення прийде сюди, у цей чат.
        </p>
      ) : (
        <p className='text-body text-ink-2'>Найближчим часом з Вами зв’яжеться наш адвокат.</p>
      )}
      <Button variant='quiet' onClick={onAgain}>
        Надіслати ще одну
      </Button>
    </Card>
  )
}
