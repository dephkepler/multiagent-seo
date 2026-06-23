'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { Select } from '@/components/ui/select'

interface PlaceBacklinksAccepted {
  sheet: string
  sites_queued: number
  run_id: string
}

interface Placement {
  id: number
  donor_url: string
  target_url: string
  ok: boolean
  status: string
  post_url?: string
  edit_url?: string
  anchor?: string
  created_at: string
}

interface WordpressSite {
  id: string
  alias: string
  url: string
  enabled: boolean
}

export default function PlaceBacklinksPage() {
  const [sheet, setSheet] = useState('WEBSITES')
  const [targetSiteUrl, setTargetSiteUrl] = useState('')
  const [count, setCount] = useState(3)
  const [provider, setProvider] = useState('')

  const [runId, setRunId] = useState<string | null>(null)
  const [queued, setQueued] = useState(0)
  const [target, setTarget] = useState(0)

  const sites = useQuery({
    queryKey: ['wordpress-sites'],
    queryFn: () => api<WordpressSite[]>('/wordpress-sites'),
  })
  const siteOptions = (sites.data || []).filter((s) => s.enabled)

  useEffect(() => {
    if (!targetSiteUrl && siteOptions.length > 0) {
      setTargetSiteUrl(siteOptions[0].url)
    }
  }, [siteOptions, targetSiteUrl])

  const placements = useQuery({
    queryKey: ['placements', runId],
    queryFn: () => api<Placement[]>(`/linkbuilding/placements?run_id=${runId}`),
    enabled: runId != null,
    refetchInterval: (q) => {
      const data = (q.state.data as Placement[] | undefined) || []
      const ok = data.filter((p) => p.ok).length
      return ok >= target || data.length >= queued ? false : 2000
    },
  })

  const results = placements.data || []
  const succeeded = results.filter((p) => p.ok).length
  const done = runId != null && (succeeded >= target || results.length >= queued)

  const run = useMutation({
    mutationFn: () =>
      api<PlaceBacklinksAccepted>('/linkbuilding/place-backlinks', {
        method: 'POST',
        body: JSON.stringify({
          sheet,
          target_site_url: targetSiteUrl,
          count,
          ...(provider ? { provider } : {}),
        }),
      }),
    onSuccess: (r) => {
      setRunId(r.run_id)
      setQueued(r.sites_queued)
      setTarget(count)
      toast.success(`Started — up to ${count} placements across ${r.sites_queued} donors`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className='max-w-2xl space-y-6'>
      <Card>
        <h1 className='mb-1 text-lg font-semibold'>Place backlinks on donor sites</h1>
        <p className='mb-6 text-sm text-gray-500'>
          For each donor (url + login + password in columns E–G) it logs into WordPress, picks the latest post via WP REST, asks the LLM to weave in a contextual{' '}
          <code className='rounded bg-gray-100 px-1'>{'<a>'}</code> linking to your site, updates the post, and records the result in column I. Already-placed donors are skipped;
          it stops once it reaches the requested number of successful placements.
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
            <Label>Target site (your site, the backlink destination)</Label>
            <Select value={targetSiteUrl} onChange={(e) => setTargetSiteUrl(e.target.value)} required disabled={sites.isLoading}>
              {siteOptions.length === 0 && <option value=''>{sites.isLoading ? 'Loading…' : 'No sites configured'}</option>}
              {siteOptions.map((s) => (
                <option key={s.id} value={s.url}>
                  {s.alias} — {s.url}
                </option>
              ))}
            </Select>
          </div>
          <div>
            <Label>How many to place today (stops after this many successful)</Label>
            <Input type='number' min={1} value={count} onChange={(e) => setCount(Math.max(1, Math.floor(+e.target.value) || 1))} required />
          </div>
          <div>
            <Label>Provider (optional)</Label>
            <Select value={provider} onChange={(e) => setProvider(e.target.value)}>
              <option value=''>Server default</option>
              <option value='groq'>Groq</option>
              <option value='claude'>Claude</option>
            </Select>
          </div>
          <Button type='submit' disabled={run.isPending || !targetSiteUrl}>
            {run.isPending ? 'Queuing…' : 'Start placement'}
          </Button>
        </form>
      </Card>

      {runId && (
        <Card>
          <div className='mb-2 flex items-center justify-between text-sm'>
            <span className='font-medium'>{done ? 'Done' : 'Placing…'}</span>
            <span className='text-gray-500'>
              {succeeded}/{target} placed · {results.length} tried
            </span>
          </div>
          <div className='mb-4 h-2 w-full overflow-hidden rounded bg-gray-100'>
            <div className='h-2 rounded bg-emerald-500 transition-all' style={{ width: `${Math.min(100, (succeeded / Math.max(1, target)) * 100)}%` }} />
          </div>
          <ul className='space-y-1 text-sm'>
            {results.length === 0 && <li className='text-gray-400'>Waiting for the first result…</li>}
            {results.map((p) => {
              const link = p.post_url || p.edit_url
              return (
                <li key={p.id} className='flex items-center justify-between gap-3 border-b border-gray-50 py-1'>
                  <span className='truncate'>
                    {p.ok ? '✅' : '❌'} <span className='text-gray-700'>{p.donor_url}</span>
                  </span>
                  {p.ok && link ? (
                    <a href={link} target='_blank' rel='noreferrer' className='shrink-0 text-sky-600 hover:underline'>
                      article ↗
                    </a>
                  ) : (
                    <span className='shrink-0 max-w-[55%] truncate text-xs text-gray-400' title={p.status}>
                      {p.status}
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        </Card>
      )}
    </div>
  )
}
