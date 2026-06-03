'use client'

import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'

interface LoginAccepted {
  sheet: string
  sites_queued: number
}

export default function AutoLoginPage() {
  const [sheet, setSheet] = useState('WEBSITES')
  // Empty input = no topic filter, log into every D=yes donor.
  const [topics, setTopics] = useState('')

  const run = useMutation({
    mutationFn: () => {
      const list = topics
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)
      return api<LoginAccepted>('/linkbuilding/login', {
        method: 'POST',
        body: JSON.stringify({ sheet, ...(list.length ? { topics: list } : {}) }),
      })
    },
    onSuccess: (r) => toast.success(`Queued ${r.sites_queued} sites in ${r.sheet}`),
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className='max-w-2xl space-y-6'>
      <Card>
        <h1 className='mb-1 text-lg font-semibold'>Auto-login to donor sites</h1>
        <p className='mb-6 text-sm text-gray-500'>
          Reads <code className='rounded bg-gray-100 px-1'>suitable / url / login / password</code> from columns D–G, logs into each WordPress admin and writes the
          status to H. Math captchas are solved; real CAPTCHA → <code className='rounded bg-gray-100 px-1'>captcha (manual)</code>.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            run.mutate()
          }}
          className='space-y-4'
        >
          <div>
            <Label>Sheet tab</Label>
            <Input value={sheet} onChange={(e) => setSheet(e.target.value)} required />
          </div>
          <div>
            <Label>Topics filter (optional, comma-separated)</Label>
            <Input value={topics} onChange={(e) => setTopics(e.target.value)} placeholder='education, tech' />
          </div>
          <Button type='submit' disabled={run.isPending}>
            {run.isPending ? 'Queuing…' : 'Start auto-login'}
          </Button>
        </form>
      </Card>
    </div>
  )
}
