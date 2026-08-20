'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { SectionHeader } from '@/components/ui/section-header'
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
  outcome?: string
  status: string
  post_url?: string
  edit_url?: string
  anchor?: string
  created_at: string
}

interface PlacementList {
  items: Placement[]
  total: number
}

interface WordpressSite {
  id: string
  alias: string
  url: string
  enabled: boolean
}

const HISTORY_PAGE = 20

export default function PlaceBacklinksPage() {
  const [sheet, setSheet] = useState('WEBSITES')
  const [targetSiteUrl, setTargetSiteUrl] = useState('')
  const [count, setCount] = useState(3)
  const [provider, setProvider] = useState('')

  const [runId, setRunId] = useState<string | null>(null)
  const [queued, setQueued] = useState(0)
  const [target, setTarget] = useState(0)
  const [canceled, setCanceled] = useState(false)

  const [historyOffset, setHistoryOffset] = useState(0)

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
      if (canceled) return false
      const data = (q.state.data as Placement[] | undefined) || []
      const ok = data.filter((p) => p.ok).length
      return ok >= target || data.length >= queued ? false : 2000
    },
  })

  const results = placements.data || []
  const succeeded = results.filter((p) => p.ok).length
  const done = runId != null && (canceled || succeeded >= target || results.length >= queued)

  const history = useQuery({
    queryKey: ['placement-history', historyOffset],
    queryFn: () => api<PlacementList>(`/linkbuilding/placements/history?limit=${HISTORY_PAGE}&offset=${historyOffset}`),
  })

  useEffect(() => {
    if (done) history.refetch()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [done])

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
      setCanceled(false)
      setRunId(r.run_id)
      setQueued(r.sites_queued)
      setTarget(count)
      toast.success(`Started — up to ${count} placements across ${r.sites_queued} donors`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const cancel = useMutation({
    mutationFn: () => api(`/linkbuilding/placements/${runId}/cancel`, { method: 'POST' }),
    onSuccess: () => {
      setCanceled(true)
      toast.success('Cancellation requested')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const total = history.data?.total || 0

  return (
    <div className='max-w-2xl space-y-6'>
      <Card>
        <h1 className='mb-1 text-lg font-semibold'>Place backlinks on donor sites</h1>
        <p className='mb-6 text-sm text-gray-500'>
          For each donor (url + login + password in columns E–G) it logs into WordPress, picks the latest post via WP REST, asks the LLM to weave in a contextual{' '}
          <code className='rounded bg-gray-100 px-1'>{'<a>'}</code> linking to your site, updates the post, and records the result in columns H–I. Already-placed and
          permanently-blocked donors are skipped; it stops once it reaches the requested number of successful placements.
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
          <SectionHeader
            title={canceled ? 'Canceled' : done ? 'Done' : 'Placing…'}
            action={
              <div className='flex items-center gap-3'>
                <span className='text-sm text-gray-500'>
                  {succeeded}/{target} placed · {results.length} tried
                </span>
                {!done && (
                  <Button type='button' variant='secondary' size='sm' onClick={() => cancel.mutate()} disabled={cancel.isPending}>
                    {cancel.isPending ? 'Canceling…' : 'Cancel'}
                  </Button>
                )}
              </div>
            }
          />
          <div className='mb-4 h-2 w-full overflow-hidden rounded bg-gray-100'>
            <div className='h-2 rounded bg-emerald-500 transition-all' style={{ width: `${Math.min(100, (succeeded / Math.max(1, target)) * 100)}%` }} />
          </div>
          <ul className='space-y-1 text-sm'>
            {results.length === 0 && <li className='text-gray-400'>Waiting for the first result…</li>}
            {results.map((p) => {
              const pending = p.outcome === 'pending'
              return (
                <li key={p.id} className='flex items-center justify-between gap-2 border-b border-gray-50 py-1.5'>
                  <div className='flex min-w-0 items-center gap-2'>
                    <Badge variant={p.ok ? 'success' : pending ? 'warning' : 'danger'}>{p.ok ? 'Placed' : pending ? 'Pending' : 'Failed'}</Badge>
                    <span className='truncate text-gray-700' title={p.donor_url}>
                      {p.donor_url}
                    </span>
                  </div>
                  {p.ok && (p.post_url || p.edit_url) ? (
                    <a href={p.post_url || p.edit_url} target='_blank' rel='noreferrer' className='shrink-0 text-sky-600 hover:underline'>
                      article ↗
                    </a>
                  ) : pending && p.edit_url ? (
                    <a href={p.edit_url} target='_blank' rel='noreferrer' className='shrink-0 text-amber-600 hover:underline' title={p.status}>
                      edit/publish ↗
                    </a>
                  ) : (
                    <span className='max-w-[45%] shrink-0 truncate text-xs text-gray-400' title={p.status}>
                      {p.status}
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        </Card>
      )}

      <Card>
        <SectionHeader title='Placed before' action={<span className='text-sm text-gray-500'>{total} total</span>} />
        <ul className='space-y-1 text-sm'>
          {history.isLoading && <li className='text-gray-400'>Loading…</li>}
          {!history.isLoading && (history.data?.items.length || 0) === 0 && <li className='text-gray-400'>No successful placements yet.</li>}
          {(history.data?.items || []).map((p) => {
            const link = p.post_url || p.edit_url
            return (
              <li key={p.id} className='flex items-center justify-between gap-3 border-b border-gray-50 py-1'>
                <span className='min-w-0'>
                  <span className='block truncate text-gray-700' title={p.donor_url}>
                    {p.donor_url}
                  </span>
                  <span className='block truncate text-xs text-gray-400' title={`${p.target_url} · ${p.created_at.slice(0, 10)}`}>
                    → {p.target_url} · {p.created_at.slice(0, 10)}
                  </span>
                </span>
                {link && (
                  <a href={link} target='_blank' rel='noreferrer' className='shrink-0 text-sky-600 hover:underline'>
                    article ↗
                  </a>
                )}
              </li>
            )
          })}
        </ul>
        {total > HISTORY_PAGE && (
          <div className='mt-3 flex items-center justify-between text-sm'>
            <Button type='button' variant='secondary' size='sm' onClick={() => setHistoryOffset((o) => Math.max(0, o - HISTORY_PAGE))} disabled={historyOffset === 0}>
              ← Prev
            </Button>
            <span className='text-gray-500'>
              {historyOffset + 1}–{Math.min(historyOffset + HISTORY_PAGE, total)} of {total}
            </span>
            <Button type='button' variant='secondary' size='sm' onClick={() => setHistoryOffset((o) => o + HISTORY_PAGE)} disabled={historyOffset + HISTORY_PAGE >= total}>
              Next →
            </Button>
          </div>
        )}
      </Card>
    </div>
  )
}
