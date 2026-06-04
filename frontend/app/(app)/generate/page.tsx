'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
}

export default function GeneratePage() {
  const qc = useQueryClient()
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
    queryKey: ['articles'],
    queryFn: () => api<Article[] | { items: Article[] }>('/articles'),
    refetchInterval: 5_000,
  })
  const items: Article[] = Array.isArray(articles.data) ? articles.data : (articles.data as any)?.items || []

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

  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])

  const sitesById = new Map((sites.data || []).map((s) => [s.id, s]))

  function on<K extends keyof GenerateRequest>(k: K, v: GenerateRequest[K]) {
    setForm((f) => ({ ...f, [k]: v }))
  }

  return (
    <div className='space-y-6'>
      <Card>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate(form)
          }}
          className='grid grid-cols-1 gap-4 md:grid-cols-3 lg:grid-cols-5'
        >
          <Field label='Keyword' className='md:col-span-2'>
            <Input value={form.keyword} onChange={(e) => on('keyword', e.target.value)} required />
          </Field>
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
            <Button type='submit' disabled={create.isPending} className='px-10'>
              {create.isPending ? 'Queuing…' : 'Create'}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <div className='mb-4 flex items-center justify-between'>
          <h2 className='text-base font-semibold'>Articles</h2>
          <div className='flex items-center gap-3'>
            <span className='text-xs text-gray-500'>
              {articles.dataUpdatedAt ? `refreshed ${fmtAgo(now, articles.dataUpdatedAt)}` : '—'}
            </span>
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
                <th className='py-2'>WordPress</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 && (
                <tr>
                  <td colSpan={7} className='py-6 text-center text-gray-400'>
                    {articles.isLoading ? 'Loading…' : 'No articles yet'}
                  </td>
                </tr>
              )}
              {items.map((a) => {
                const site = a.site_id ? sitesById.get(a.site_id) : undefined
                const terminal = ['draft', 'published', 'failed'].includes(a.status)
                const elapsed = fmtElapsed(a.created_at, a.updated_at, now, terminal)
                return (
                  <tr key={String(a.id)} className='border-t border-gray-100'>
                    <td className='py-2 pr-4 text-gray-500'>{a.id}</td>
                    <td className='py-2 pr-4'>{a.keyword}</td>
                    <td className='py-2 pr-4 text-gray-500'>{site?.alias || '—'}</td>
                    <td className='py-2 pr-4'>
                      <StatusPill status={a.status} />
                    </td>
                    <td className='py-2 pr-4 text-gray-500'>{new Date(a.created_at).toLocaleString()}</td>
                    <td className='py-2 pr-4 text-gray-500'>{elapsed}</td>
                    <td className='py-2'>
                      <WpActions article={a} siteUrl={site?.url} onPublish={(id) => publish.mutate(id)} publishing={publish.isPending} />
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </Card>
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

function fmtAgo(now: number, t: number): string {
  const s = Math.max(0, Math.floor((now - t) / 1000))
  if (s < 60) return `${s}s ago`
  return `${Math.floor(s / 60)}m ${s % 60}s ago`
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

function StatusPill({ status }: { status: string }) {
  const colour =
    status === 'published' ? 'bg-emerald-100 text-emerald-800' : status === 'draft' ? 'bg-sky-100 text-sky-800' : status === 'failed' ? 'bg-rose-100 text-rose-800' : 'bg-amber-100 text-amber-800'
  return <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${colour}`}>{status}</span>
}
