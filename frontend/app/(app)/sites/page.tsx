'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'
import { Select } from '@/components/ui/select'

type Provider = 'wordpress' | 'modx'

interface LanguageConfig {
  contextKey?: string
  keywordsSpreadsheetId?: string
  keywordsSheet?: string
}

interface Site {
  id: string
  alias: string
  provider: Provider
  url: string
  username: string
  languages?: Record<string, LanguageConfig>
  enabled: boolean
  createdAt: string
  updatedAt: string
}

interface CreateRequest {
  alias: string
  provider: Provider
  url: string
  username: string
  appPassword: string
}

type LanguageRow = { code: string; contextKey: string; keywordsSpreadsheetId: string; keywordsSheet: string }

const emptyLanguageRow: LanguageRow = { code: '', contextKey: '', keywordsSpreadsheetId: '', keywordsSheet: '' }

export default function SitesPage() {
  const qc = useQueryClient()
  const [languagesOpenFor, setLanguagesOpenFor] = useState<string | null>(null)

  const sites = useQuery({
    queryKey: ['wordpress-sites'],
    queryFn: () => api<Site[]>('/wordpress-sites'),
  })

  const create = useMutation({
    mutationFn: (body: CreateRequest) => api<Site>('/wordpress-sites', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Site added')
      qc.invalidateQueries({ queryKey: ['wordpress-sites'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Record<string, unknown> }) =>
      api<Site>(`/wordpress-sites/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['wordpress-sites'] }),
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) => api(`/wordpress-sites/${id}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('Site removed')
      qc.invalidateQueries({ queryKey: ['wordpress-sites'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className='space-y-6'>
      <Card>
        <h1 className='mb-4 text-lg font-semibold'>Add site</h1>
        <AddForm onSubmit={(body) => create.mutate(body)} busy={create.isPending} />
      </Card>

      <Card>
        <div className='mb-4 flex items-center justify-between'>
          <h2 className='text-base font-semibold'>Sites</h2>
          <Button variant='secondary' size='sm' onClick={() => sites.refetch()}>
            Refresh
          </Button>
        </div>
        <div className='overflow-x-auto'>
          <table className='w-full text-sm'>
            <thead className='text-left text-xs uppercase text-gray-500'>
              <tr>
                <th className='py-2 pr-4'>Alias</th>
                <th className='py-2 pr-4'>Provider</th>
                <th className='py-2 pr-4'>URL</th>
                <th className='py-2 pr-4'>Login</th>
                <th className='py-2 pr-4'>Languages</th>
                <th className='py-2 pr-4'>Enabled</th>
                <th className='py-2'>Actions</th>
              </tr>
            </thead>
            <tbody>
              {(sites.data || []).length === 0 && (
                <tr>
                  <td colSpan={7} className='py-6 text-center text-gray-400'>
                    {sites.isLoading ? 'Loading…' : 'No sites yet — add one above'}
                  </td>
                </tr>
              )}
              {(sites.data || []).map((s) => (
                <>
                  <tr key={s.id} className='border-t border-gray-100'>
                    <td className='py-2 pr-4 font-medium'>{s.alias}</td>
                    <td className='py-2 pr-4 text-gray-500'>{s.provider}</td>
                    <td className='py-2 pr-4'>
                      <a className='text-emerald-700 hover:underline' href={s.url} target='_blank' rel='noreferrer'>
                        {s.url}
                      </a>
                    </td>
                    <td className='py-2 pr-4 text-gray-500'>{s.username || '—'}</td>
                    <td className='py-2 pr-4'>
                      <button
                        onClick={() => setLanguagesOpenFor(languagesOpenFor === s.id ? null : s.id)}
                        className='text-sky-700 hover:underline'
                      >
                        {Object.keys(s.languages || {}).length > 0 ? Object.keys(s.languages!).join(', ') : 'set up…'}
                      </button>
                    </td>
                    <td className='py-2 pr-4'>
                      <button
                        onClick={() => update.mutate({ id: s.id, body: { enabled: !s.enabled } })}
                        className={`rounded px-2 py-0.5 text-xs font-medium ${
                          s.enabled ? 'bg-emerald-100 text-emerald-800' : 'bg-gray-100 text-gray-600'
                        }`}
                      >
                        {s.enabled ? 'enabled' : 'disabled'}
                      </button>
                    </td>
                    <td className='py-2'>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => {
                          if (confirm(`Remove site "${s.alias}"?`)) del.mutate(s.id)
                        }}
                        className='text-rose-600 hover:bg-rose-50'
                      >
                        Delete
                      </Button>
                    </td>
                  </tr>
                  {languagesOpenFor === s.id && (
                    <tr key={`${s.id}-languages`} className='border-t border-gray-100 bg-gray-50/60'>
                      <td colSpan={7} className='p-4'>
                        <LanguagesEditor
                          site={s}
                          busy={update.isPending}
                          onSave={(languages) => {
                            update.mutate({ id: s.id, body: { languages } })
                            setLanguagesOpenFor(null)
                          }}
                          onCancel={() => setLanguagesOpenFor(null)}
                        />
                      </td>
                    </tr>
                  )}
                </>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}

function LanguagesEditor({
  site,
  busy,
  onSave,
  onCancel,
}: {
  site: Site
  busy: boolean
  onSave: (languages: Record<string, LanguageConfig>) => void
  onCancel: () => void
}) {
  const initial: LanguageRow[] = Object.entries(site.languages || {}).map(([code, cfg]) => ({
    code,
    contextKey: cfg.contextKey || '',
    keywordsSpreadsheetId: cfg.keywordsSpreadsheetId || '',
    keywordsSheet: cfg.keywordsSheet || '',
  }))
  const [rows, setRows] = useState<LanguageRow[]>(initial.length > 0 ? initial : [{ ...emptyLanguageRow }])

  function setRow(i: number, patch: Partial<LanguageRow>) {
    setRows((r) => r.map((row, idx) => (idx === i ? { ...row, ...patch } : row)))
  }

  return (
    <div className='space-y-3'>
      <p className='text-xs font-medium text-gray-600'>
        Languages this site generates for — for MODX, &ldquo;Context key&rdquo; is the site&apos;s own context (e.g.
        uk / web); the spreadsheet fields are optional and override the shared default keyword sheet.
      </p>
      {rows.map((row, i) => (
        <div key={i} className='grid grid-cols-1 gap-2 rounded-md border border-gray-200 bg-white p-3 md:grid-cols-5'>
          <div>
            <Label>Language code</Label>
            <Input value={row.code} onChange={(e) => setRow(i, { code: e.target.value })} placeholder='uk | ru' />
          </div>
          {site.provider === 'modx' && (
            <div>
              <Label>Context key</Label>
              <Input value={row.contextKey} onChange={(e) => setRow(i, { contextKey: e.target.value })} placeholder='uk | web' />
            </div>
          )}
          <div>
            <Label>Spreadsheet ID</Label>
            <Input value={row.keywordsSpreadsheetId} onChange={(e) => setRow(i, { keywordsSpreadsheetId: e.target.value })} />
          </div>
          <div>
            <Label>Sheet (tab) name</Label>
            <Input value={row.keywordsSheet} onChange={(e) => setRow(i, { keywordsSheet: e.target.value })} placeholder='адвокат кредит' />
          </div>
          <div className='flex items-end'>
            <Button type='button' variant='ghost' size='sm' onClick={() => setRows((r) => r.filter((_, idx) => idx !== i))}>
              Remove
            </Button>
          </div>
        </div>
      ))}
      <div className='flex items-center justify-between'>
        <Button type='button' variant='ghost' size='sm' onClick={() => setRows((r) => [...r, { ...emptyLanguageRow }])}>
          + Add language
        </Button>
        <div className='space-x-2'>
          <Button type='button' variant='secondary' size='sm' onClick={onCancel}>
            Cancel
          </Button>
          <Button
            type='button'
            size='sm'
            disabled={busy}
            onClick={() => {
              const languages: Record<string, LanguageConfig> = {}
              for (const row of rows) {
                if (!row.code.trim()) continue
                languages[row.code.trim()] = {
                  contextKey: row.contextKey || undefined,
                  keywordsSpreadsheetId: row.keywordsSpreadsheetId || undefined,
                  keywordsSheet: row.keywordsSheet || undefined,
                }
              }
              onSave(languages)
            }}
          >
            {busy ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </div>
    </div>
  )
}

const emptyForm: CreateRequest = {
  alias: '',
  provider: 'wordpress',
  url: '',
  username: '',
  appPassword: '',
}

function AddForm({ onSubmit, busy }: { onSubmit: (body: CreateRequest) => void; busy: boolean }) {
  const [form, setForm] = useState<CreateRequest>(emptyForm)
  function set<K extends keyof CreateRequest>(k: K, v: CreateRequest[K]) {
    setForm((f) => ({ ...f, [k]: v }))
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(form)
        setForm(emptyForm)
      }}
      className='space-y-4'
    >
      <div className='grid grid-cols-1 gap-4 md:grid-cols-3'>
        <div>
          <Label>Alias</Label>
          <Input value={form.alias} onChange={(e) => set('alias', e.target.value)} placeholder='abalis' required />
        </div>
        <div>
          <Label>Provider</Label>
          <Select value={form.provider} onChange={(e) => set('provider', e.target.value as Provider)}>
            <option value='wordpress'>WordPress</option>
            <option value='modx'>MODX</option>
          </Select>
        </div>
        <div>
          <Label>URL</Label>
          <Input
            type='url'
            value={form.url}
            onChange={(e) => set('url', e.target.value)}
            placeholder='https://example.com'
            required
          />
        </div>
      </div>

      {form.provider === 'wordpress' ? (
        <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
          <div>
            <Label>Username</Label>
            <Input value={form.username} onChange={(e) => set('username', e.target.value)} placeholder='admin' required />
          </div>
          <div>
            <Label>App password</Label>
            <Input
              type='password'
              value={form.appPassword}
              onChange={(e) => set('appPassword', e.target.value)}
              placeholder='xxxx xxxx xxxx xxxx'
              required
            />
          </div>
        </div>
      ) : (
        <div>
          <Label>API key</Label>
          <Input
            type='password'
            value={form.appPassword}
            onChange={(e) => set('appPassword', e.target.value)}
            placeholder='agentapi_key from MODX Manager'
            required
          />
        </div>
      )}

      <p className='text-xs text-gray-400'>
        {form.provider === 'modx'
          ? 'After adding, set up its languages/contexts from the site list below before generating.'
          : 'Languages are optional — leave them unset and this site uses the shared default keyword sheet.'}
      </p>

      <Button type='submit' disabled={busy}>
        {busy ? 'Adding…' : 'Add site'}
      </Button>
    </form>
  )
}
