'use client'

import { useEffect, useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { Select } from '@/components/ui/select'

interface GenerateRequest {
  keyword: string
  site_id: string
  provider?: string
  language?: string
  min_words?: number
  max_words?: number
  max_tokens?: number
  max_cycles?: number
  ai_threshold?: number
  auto_publish?: boolean
  include_images?: boolean
}

interface WordpressSite {
  id: string
  alias: string
  url: string
  enabled: boolean
}

interface Article {
  id: number
  keyword: string
  site_id?: string
  status: string
  created_at: string
  updated_at?: string
  wp_edit_url?: string
  wp_post_id?: number
  human_rating?: boolean | null
}

interface ArticleDetail extends Article {
  published_at?: string
  ai_score?: number
  reward?: number
  quality_ok?: boolean
  humanize_cycles?: number
  tokens?: number
  images_requested?: number
  images_resolved?: number
  images_skipped?: number
  request_params?: Record<string, any>
  check_result?: Record<string, any>
}

// Статусы, после которых статья больше не меняется — на них опрос /articles можно остановить.
const TERMINAL_STATUSES = ['draft', 'published', 'failed']

export default function GeneratePage() {
  const qc = useQueryClient()
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(25)
  const [massMode, setMassMode] = useState(false)
  const [count, setCount] = useState(30)
  const [infoId, setInfoId] = useState<number | null>(null)
  const [form, setForm] = useState<GenerateRequest>({
    keyword: '',
    site_id: '',
    provider: 'groq',
    language: 'en',
    min_words: 200,
    max_words: 500,
    max_tokens: 400,
    max_cycles: 1,
    ai_threshold: 0.8,
    auto_publish: false,
    include_images: true,
  })

  const sites = useQuery({
    queryKey: ['wordpress-sites'],
    queryFn: () => api<WordpressSite[]>('/wordpress-sites'),
  })
  const siteOptions = (sites.data || []).filter((s) => s.enabled)

  useEffect(() => {
    if (!form.site_id && siteOptions.length > 0) {
      setForm((f) => ({ ...f, site_id: siteOptions[0].id }))
    }
  }, [siteOptions, form.site_id])

  const articles = useQuery({
    queryKey: ['articles', page, pageSize],
    queryFn: () => api<{ items: Article[]; total: number }>(`/articles?limit=${pageSize}&offset=${page * pageSize}`),
    refetchInterval: (query) => {
      const list = query.state.data?.items || []
      const busy = list.some((a) => !TERMINAL_STATUSES.includes(a.status))
      return busy ? 5_000 : false
    },
    // Idle articles never change on their own; mutations invalidate explicitly
    // and the busy-poll above covers generations. Without this the 1s elapsed
    // timer's re-renders trip the global 30s staleTime into background refetches.
    staleTime: Infinity,
    placeholderData: keepPreviousData,
  })
  const items: Article[] = articles.data?.items || []
  const total = articles.data?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const busy = items.some((a) => !TERMINAL_STATUSES.includes(a.status))

  const create = useMutation({
    mutationFn: (body: GenerateRequest) =>
      api<{ article_id: string }>('/generate', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Queued')
      qc.invalidateQueries({ queryKey: ['articles'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const publish = useMutation({
    mutationFn: (id: number) => api(`/articles/${id}/publish`, { method: 'POST' }),
    onSuccess: () => {
      toast.success('Publish queued')
      qc.invalidateQueries({ queryKey: ['articles'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const rate = useMutation({
    mutationFn: ({ id, rating }: { id: number; rating: 'like' | 'dislike' | 'none' }) =>
      api(`/articles/${id}/rating`, { method: 'POST', body: JSON.stringify({ rating }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['articles'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  const batch = useMutation({
    mutationFn: (body: { count: number } & Partial<GenerateRequest>) =>
      api<{ count: number; article_ids: number[] }>('/generate/batch', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: (res) => {
      toast.success(`Queued ${res.count} articles`)
      qc.invalidateQueries({ queryKey: ['articles'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const detail = useQuery({
    queryKey: ['article', infoId],
    queryFn: () => api<ArticleDetail>(`/articles/${infoId}`),
    enabled: infoId != null,
    staleTime: 0,
    refetchOnMount: 'always',
  })

  useEffect(() => {
    if (infoId == null) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setInfoId(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [infoId])

  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!busy) return
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [busy])

  const sitesById = new Map((sites.data || []).map((s) => [s.id, s]))

  function on<K extends keyof GenerateRequest>(k: K, v: GenerateRequest[K]) {
    setForm((f) => ({ ...f, [k]: v }))
  }

  return (
    <div className='space-y-6'>
      <Card>
        <div className='mb-4 flex items-center gap-2 text-sm'>
          <button
            type='button'
            onClick={() => setMassMode(false)}
            className={`rounded px-3 py-1 ${!massMode ? 'bg-sky-100 text-sky-800 font-medium' : 'text-gray-500 hover:bg-gray-100'}`}
          >
            Single
          </button>
          <button
            type='button'
            onClick={() => setMassMode(true)}
            className={`rounded px-3 py-1 ${massMode ? 'bg-sky-100 text-sky-800 font-medium' : 'text-gray-500 hover:bg-gray-100'}`}
          >
            Mass
          </button>
        </div>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (massMode) {
              const { keyword, ...shared } = form
              const n = Math.max(1, Math.min(100, Math.floor(count) || 1))
              batch.mutate({ count: n, ...shared })
            } else {
              create.mutate(form)
            }
          }}
          className='grid grid-cols-1 gap-4 md:grid-cols-3 lg:grid-cols-5'
        >
          {massMode ? (
            <Field label='Number of articles' className='md:col-span-2'>
              <Input type='number' min={1} max={100} value={count} onChange={(e) => setCount(+e.target.value)} required />
            </Field>
          ) : (
            <Field label='Keyword' className='md:col-span-2'>
              <Input value={form.keyword} onChange={(e) => on('keyword', e.target.value)} required />
            </Field>
          )}
          <Field label='Site' className='md:col-span-2'>
            <Select value={form.site_id} onChange={(e) => on('site_id', e.target.value)} required disabled={sites.isLoading}>
              {siteOptions.length === 0 && <option value=''>{sites.isLoading ? 'Loading…' : 'No sites configured'}</option>}
              {siteOptions.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.alias}
                </option>
              ))}
            </Select>
          </Field>
          <Field label='Language'>
            <Select value={form.language} onChange={(e) => on('language', e.target.value)}>
              <option value='en'>English</option>
              <option value='ru'>Russian</option>
              <option value='uk'>Ukrainian</option>
            </Select>
          </Field>
          <Field label='Provider'>
            <Select value={form.provider} onChange={(e) => on('provider', e.target.value)}>
              <option value='groq'>Groq</option>
              <option value='claude'>Claude</option>
            </Select>
          </Field>
          <Field label='Min words'>
            <Input type='number' value={form.min_words} onChange={(e) => on('min_words', +e.target.value)} />
          </Field>
          <Field label='Max words'>
            <Input type='number' value={form.max_words} onChange={(e) => on('max_words', +e.target.value)} />
          </Field>
          <Field label='Max tokens'>
            <Input type='number' value={form.max_tokens} onChange={(e) => on('max_tokens', +e.target.value)} />
          </Field>
          <Field label='Cycles'>
            <Input type='number' value={form.max_cycles} onChange={(e) => on('max_cycles', +e.target.value)} />
          </Field>
          <Field label='AI threshold'>
            <Input type='number' step='0.1' value={form.ai_threshold} onChange={(e) => on('ai_threshold', +e.target.value)} />
          </Field>
          <Field label='Auto-publish'>
            <Select value={form.auto_publish ? 'yes' : 'no'} onChange={(e) => on('auto_publish', e.target.value === 'yes')}>
              <option value='no'>No</option>
              <option value='yes'>Yes</option>
            </Select>
          </Field>
          <Field label='Images'>
            <Select value={form.include_images ? 'yes' : 'no'} onChange={(e) => on('include_images', e.target.value === 'yes')}>
              <option value='yes'>Yes</option>
              <option value='no'>No</option>
            </Select>
          </Field>
          <div className='md:col-span-3 lg:col-span-5 flex justify-center pt-2'>
            <Button type='submit' disabled={create.isPending || batch.isPending} className='px-10'>
              {massMode ? (batch.isPending ? 'Queuing…' : `Generate ${count}`) : create.isPending ? 'Queuing…' : 'Create'}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <div className='mb-4 flex items-center justify-between'>
          <h2 className='text-base font-semibold'>Articles</h2>
          <div className='flex items-center gap-2'>
            <select
              value={pageSize}
              onChange={(e) => {
                setPageSize(+e.target.value)
                setPage(0)
              }}
              className='rounded border border-gray-200 px-2 py-1 text-sm'
            >
              <option value={25}>25 / page</option>
              <option value={50}>50 / page</option>
              <option value={100}>100 / page</option>
            </select>
            <Button variant='secondary' size='sm' onClick={() => articles.refetch()}>
              Refresh
            </Button>
          </div>
        </div>
        <div className='overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead className='text-left text-xs uppercase text-gray-500'>
              <tr>
                <th className='py-2 pr-4'>ID</th>
                <th className='py-2 pr-4'>Keyword</th>
                <th className='py-2 pr-4'>Site</th>
                <th className='py-2 pr-4'>Status</th>
                <th className='py-2 pr-4'>Created</th>
                <th className='py-2 pr-4'>Elapsed</th>
                <th className='py-2 pr-4'>Rating</th>
                <th className='py-2'>WordPress</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 && (
                <tr>
                  <td colSpan={8} className='py-6 text-center text-gray-400'>
                    {articles.isLoading ? 'Loading…' : 'No articles yet'}
                  </td>
                </tr>
              )}
              {items.map((a) => {
                const site = a.site_id ? sitesById.get(a.site_id) : undefined
                const terminal = TERMINAL_STATUSES.includes(a.status)
                const elapsed = fmtElapsed(a.created_at, a.updated_at, now, terminal)
                return (
                  <tr key={String(a.id)} className='border-t border-gray-100'>
                    <td className='py-2 pr-4'>
                      <button onClick={() => setInfoId(a.id)} className='text-sky-700 hover:underline' title='Info'>
                        {a.id}
                      </button>
                    </td>
                    <td className='py-2 pr-4'>{a.keyword}</td>
                    <td className='py-2 pr-4 text-gray-500'>{site?.alias || '—'}</td>
                    <td className='py-2 pr-4'>
                      <StatusPill status={a.status} />
                    </td>
                    <td className='py-2 pr-4 text-gray-500'>{new Date(a.created_at).toLocaleString()}</td>
                    <td className='py-2 pr-4 text-gray-500'>{elapsed}</td>
                    <td className='py-2 pr-4'>
                      <RatingButtons rating={a.human_rating} onRate={(r) => rate.mutate({ id: a.id, rating: r })} disabled={rate.isPending} />
                    </td>
                    <td className='py-2'>
                      <WpActions article={a} siteUrl={site?.url} onPublish={(id) => publish.mutate(id)} publishing={publish.isPending} />
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <div className='mt-4 flex items-center justify-between text-sm text-gray-500'>
          <span>{total} total</span>
          <div className='flex items-center gap-3'>
            <button
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={page === 0}
              className='hover:underline disabled:opacity-40'
            >
              ← Prev
            </button>
            <span>
              Page {page + 1} / {totalPages}
            </span>
            <button
              onClick={() => setPage((p) => p + 1)}
              disabled={page + 1 >= totalPages}
              className='hover:underline disabled:opacity-40'
            >
              Next →
            </button>
          </div>
        </div>
      </Card>

      {infoId != null && (
        <div className='fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4' onClick={() => setInfoId(null)}>
          <div
            className='max-h-[85vh] w-full max-w-2xl overflow-auto rounded-lg bg-white p-6 shadow-xl'
            onClick={(e) => e.stopPropagation()}
          >
            <div className='mb-4 flex items-center justify-between'>
              <h3 className='text-base font-semibold'>Article #{infoId}</h3>
              <button onClick={() => setInfoId(null)} className='text-gray-400 hover:text-gray-700'>
                ✕
              </button>
            </div>
            {detail.isLoading ? (
              <p className='text-gray-400'>Loading…</p>
            ) : detail.data ? (
              <InfoBody a={detail.data} />
            ) : (
              <p className='text-rose-600'>Failed to load</p>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function InfoBody({ a }: { a: ArticleDetail }) {
  const rp = a.request_params || {}
  const flagged: string[] = (a.check_result?.sentences_flagged as string[]) || []
  const Row = ({ k, v }: { k: string; v: React.ReactNode }) =>
    v == null || v === '' ? null : (
      <div className='flex justify-between gap-4 border-b border-gray-50 py-1'>
        <span className='text-gray-500'>{k}</span>
        <span className='text-right'>{v}</span>
      </div>
    )
  return (
    <div className='space-y-4 text-sm'>
      <section>
        <h4 className='mb-1 font-medium text-gray-700'>Overview</h4>
        <Row k='Keyword' v={a.keyword} />
        <Row k='Status' v={a.status} />
        <Row k='Created' v={new Date(a.created_at).toLocaleString()} />
        <Row k='Published' v={a.published_at ? new Date(a.published_at).toLocaleString() : '—'} />
        <Row k='Rating' v={a.human_rating == null ? '—' : a.human_rating ? '👍' : '👎'} />
      </section>
      <section>
        <h4 className='mb-1 font-medium text-gray-700'>Metrics</h4>
        <Row k='AI score' v={a.ai_score == null ? undefined : a.ai_score.toFixed(2)} />
        <Row k='Reward' v={a.reward == null ? undefined : a.reward.toFixed(2)} />
        <Row k='Quality floor' v={a.quality_ok == null ? '—' : a.quality_ok ? 'pass' : 'fail'} />
        <Row k='Humanize cycles' v={a.humanize_cycles} />
        <Row k='Tokens' v={a.tokens} />
        <Row k='Images' v={`${a.images_resolved ?? 0}/${a.images_requested ?? 0} (skipped ${a.images_skipped ?? 0})`} />
      </section>
      <section>
        <h4 className='mb-1 font-medium text-gray-700'>Settings</h4>
        <Row k='Provider' v={rp.provider} />
        <Row k='Model' v={rp.model} />
        <Row k='Words' v={rp.min_words && rp.max_words ? `${rp.min_words}–${rp.max_words}` : null} />
        <Row k='Max tokens' v={rp.max_tokens} />
        <Row k='Cycles' v={rp.max_cycles} />
        <Row k='AI threshold' v={rp.ai_threshold} />
        <Row k='Language' v={rp.language} />
        <Row k='Auto-publish' v={rp.auto_publish == null ? null : rp.auto_publish ? 'yes' : 'no'} />
      </section>
      {flagged.length > 0 && (
        <section>
          <h4 className='mb-1 font-medium text-gray-700'>Flagged sentences</h4>
          <ul className='list-disc space-y-1 pl-5 text-gray-600'>
            {flagged.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}

function Field({ label, children, className }: { label: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={className}>
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function fmtElapsed(start: string, end: string | undefined, now: number, terminal: boolean): string {
  const endMs = terminal && end ? new Date(end).getTime() : now
  const s = Math.max(0, Math.floor((endMs - new Date(start).getTime()) / 1000))
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function WpActions({
  article,
  siteUrl,
  onPublish,
  publishing,
}: {
  article: Article
  siteUrl?: string
  onPublish: (id: number) => void
  publishing: boolean
}) {
  if (article.status === 'failed') return <span className='text-gray-400'>—</span>
  if (!article.wp_edit_url) return <span className='text-gray-400'>—</span>

  const viewUrl = siteUrl && article.wp_post_id ? `${siteUrl.replace(/\/+$/, '')}/?p=${article.wp_post_id}` : undefined

  return (
    <span className='space-x-2 text-sm'>
      <a className='text-sky-700 hover:underline' href={article.wp_edit_url} target='_blank' rel='noreferrer'>
        edit
      </a>
      <span className='text-gray-300'>·</span>
      {article.status === 'published' && viewUrl ? (
        <a className='text-emerald-700 hover:underline' href={viewUrl} target='_blank' rel='noreferrer'>
          view
        </a>
      ) : (
        <button
          onClick={() => onPublish(article.id)}
          disabled={publishing}
          className='text-emerald-700 hover:underline disabled:opacity-50'
        >
          publish
        </button>
      )}
    </span>
  )
}

function RatingButtons({
  rating,
  onRate,
  disabled,
}: {
  rating?: boolean | null
  onRate: (rating: 'like' | 'dislike' | 'none') => void
  disabled: boolean
}) {
  return (
    <span className='space-x-1 text-base'>
      <button
        onClick={() => onRate(rating === true ? 'none' : 'like')}
        disabled={disabled}
        title={rating === true ? 'Remove like' : 'Like'}
        className={`transition ${rating === true ? 'opacity-100' : 'opacity-30 hover:opacity-100'} disabled:opacity-20`}
      >
        👍
      </button>
      <button
        onClick={() => onRate(rating === false ? 'none' : 'dislike')}
        disabled={disabled}
        title={rating === false ? 'Remove dislike' : 'Dislike'}
        className={`transition ${rating === false ? 'opacity-100' : 'opacity-30 hover:opacity-100'} disabled:opacity-20`}
      >
        👎
      </button>
    </span>
  )
}

function StatusPill({ status }: { status: string }) {
  const colour =
    status === 'published' ? 'bg-emerald-100 text-emerald-800' : status === 'draft' ? 'bg-sky-100 text-sky-800' : status === 'failed' ? 'bg-rose-100 text-rose-800' : 'bg-amber-100 text-amber-800'
  return <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${colour}`}>{status}</span>
}
