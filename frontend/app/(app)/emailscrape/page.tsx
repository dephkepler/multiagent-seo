'use client'

import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { cx } from '@/lib/cx'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { SectionHeader } from '@/components/ui/section-header'

interface EmailScrapeAccepted {
  sheet: string
  sites_queued: number
  job_id: string
}

interface ScrapeJob {
  id: string
  sheet: string
  state: string
  total: number
  processed: number
}

const jobStateBadge: Record<string, 'success' | 'danger' | 'neutral' | 'info'> = {
  running: 'info',
  done: 'success',
  canceled: 'neutral',
  failed: 'danger',
}

const jobStateCaption: Record<string, string> = {
  running: 'Working through the sheet…',
  done: 'Finished — check the sheet for results.',
  canceled: 'Stopped before finishing — already-processed rows were kept.',
  failed: 'Stopped due to a server error, not just slow — partial results may be in the sheet. Try starting a new scrape.',
}

export default function EmailScrapePage() {
  const [sheet, setSheet] = useState('DONORS')
  const [jobId, setJobId] = useState<string | null>(null)

  const start = useMutation({
    mutationFn: () =>
      api<EmailScrapeAccepted>('/emailscrape', {
        method: 'POST',
        body: JSON.stringify({ sheet }),
      }),
    onSuccess: (r) => {
      setJobId(r.job_id)
      toast.success(`Queued ${r.sites_queued} sites in ${r.sheet}`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const job = useQuery({
    queryKey: ['scrape-job', jobId],
    queryFn: () => api<ScrapeJob>(`/emailscrape/jobs/${jobId}`),
    enabled: !!jobId,
    refetchInterval: (q) => ((q.state.data as ScrapeJob | undefined)?.state === 'running' ? 2500 : false),
  })

  const cancel = useMutation({
    mutationFn: () => api(`/emailscrape/jobs/${jobId}/cancel`, { method: 'POST' }),
    onSuccess: () => toast.success('Cancellation requested'),
    onError: (e: Error) => toast.error(e.message),
  })

  const j = job.data
  const running = j?.state === 'running'
  const pct = j && j.total > 0 ? Math.round((j.processed / j.total) * 100) : 0

  return (
    <div className='max-w-2xl space-y-6'>
      <Card>
        <SectionHeader title='Scrape emails from sites' as='h1' />
        <p className='mb-6 text-sm text-gray-500'>
          Reads site URLs from column A, crawls each site (home + contact pages) and writes the found{' '}
          <code className='rounded bg-gray-100 px-1'>emails / status</code> into B–C. Already-processed rows are skipped.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            start.mutate()
          }}
          className='space-y-4'
        >
          <div>
            <Label>Sheet tab</Label>
            <Input value={sheet} onChange={(e) => setSheet(e.target.value)} required />
          </div>
          <Button type='submit' disabled={start.isPending || running}>
            {running ? 'Running…' : start.isPending ? 'Queuing…' : 'Start scrape'}
          </Button>
        </form>
      </Card>

      {jobId && job.isError && !j && (
        <Card className='border-red-200 bg-red-50 text-sm text-red-700'>
          Failed to load job status{job.error instanceof Error ? `: ${job.error.message}` : ''}.
        </Card>
      )}

      {j && (
        <Card>
          <SectionHeader
            title='Progress'
            action={
              running && (
                <Button variant='secondary' size='sm' onClick={() => cancel.mutate()} disabled={cancel.isPending}>
                  {cancel.isPending ? 'Cancelling…' : 'Cancel'}
                </Button>
              )
            }
          />
          {job.isError && <p className='mb-2 text-xs text-rose-600'>Couldn&apos;t refresh status just now — showing the last known state.</p>}
          <div className='mb-2 flex flex-wrap items-center gap-2 text-sm font-medium'>
            <span>
              {j.processed} / {j.total} sites
            </span>
            <Badge variant={jobStateBadge[j.state] ?? 'neutral'}>{j.state}</Badge>
          </div>
          <div className='h-2 w-full overflow-hidden rounded bg-gray-100'>
            <div className='h-full bg-emerald-500 transition-all' style={{ width: `${pct}%` }} />
          </div>
          <p className={cx('mt-2 text-xs', j.state === 'failed' ? 'text-rose-600' : 'text-gray-500')}>
            {jobStateCaption[j.state] ?? null}
          </p>
        </Card>
      )}
    </div>
  )
}
