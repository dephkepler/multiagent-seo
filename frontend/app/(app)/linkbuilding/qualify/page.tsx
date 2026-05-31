'use client'

import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'

interface QualifyAccepted {
  sheet: string
  websites_queued: number
}

export default function QualifyPage() {
  const [sheet, setSheet] = useState('WEBSITES')
  const [accepted, setAccepted] = useState('tech, education')
  const [candidate, setCandidate] = useState('tech, education, ecommerce, travel, casino')

  const run = useMutation({
    mutationFn: () =>
      api<QualifyAccepted>('/linkbuilding/qualify', {
        method: 'POST',
        body: JSON.stringify({
          sheet,
          accepted_topics: parseList(accepted),
          candidate_topics: parseList(candidate),
        }),
      }),
    onSuccess: (r) => toast.success(`Queued ${r.websites_queued} websites in ${r.sheet}`),
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className='max-w-2xl space-y-6'>
      <Card>
        <h1 className='mb-1 text-lg font-semibold'>Qualify donor websites</h1>
        <p className='mb-6 text-sm text-gray-500'>
          Reads URLs from column A of the sheet, classifies the topic and writes <code className='rounded bg-gray-100 px-1'>topic / outbound / suitable</code> into B–D.
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
            <Label>Accepted topics (comma-separated)</Label>
            <Input value={accepted} onChange={(e) => setAccepted(e.target.value)} required />
          </div>
          <div>
            <Label>Candidate topics (comma-separated, optional)</Label>
            <Input value={candidate} onChange={(e) => setCandidate(e.target.value)} />
          </div>
          <Button type='submit' disabled={run.isPending}>
            {run.isPending ? 'Queuing…' : 'Start qualification'}
          </Button>
        </form>
      </Card>
    </div>
  )
}

function parseList(s: string): string[] {
  return s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
}
