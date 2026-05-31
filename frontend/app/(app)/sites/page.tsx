'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'

interface WordpressSite {
  id: string
  alias: string
  url: string
  username: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

interface CreateRequest {
  alias: string
  url: string
  username: string
  appPassword: string
}

export default function SitesPage() {
  const qc = useQueryClient()
  const sites = useQuery({
    queryKey: ['wordpress-sites'],
    queryFn: () => api<WordpressSite[]>('/wordpress-sites'),
  })

  const create = useMutation({
    mutationFn: (body: CreateRequest) => api<WordpressSite>('/wordpress-sites', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      toast.success('Site added')
      qc.invalidateQueries({ queryKey: ['wordpress-sites'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<CreateRequest> & { enabled?: boolean } }) =>
      api<WordpressSite>(`/wordpress-sites/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
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
        <h1 className='mb-4 text-lg font-semibold'>Add WordPress site</h1>
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
                <th className='py-2 pr-4'>URL</th>
                <th className='py-2 pr-4'>Username</th>
                <th className='py-2 pr-4'>Enabled</th>
                <th className='py-2'>Actions</th>
              </tr>
            </thead>
            <tbody>
              {(sites.data || []).length === 0 && (
                <tr>
                  <td colSpan={5} className='py-6 text-center text-gray-400'>
                    {sites.isLoading ? 'Loading…' : 'No sites yet — add one above'}
                  </td>
                </tr>
              )}
              {(sites.data || []).map((s) => (
                <tr key={s.id} className='border-t border-gray-100'>
                  <td className='py-2 pr-4 font-medium'>{s.alias}</td>
                  <td className='py-2 pr-4'>
                    <a className='text-emerald-700 hover:underline' href={s.url} target='_blank' rel='noreferrer'>
                      {s.url}
                    </a>
                  </td>
                  <td className='py-2 pr-4 text-gray-500'>{s.username}</td>
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
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}

function AddForm({ onSubmit, busy }: { onSubmit: (body: CreateRequest) => void; busy: boolean }) {
  const [form, setForm] = useState<CreateRequest>({ alias: '', url: '', username: '', appPassword: '' })
  function set<K extends keyof CreateRequest>(k: K, v: CreateRequest[K]) {
    setForm((f) => ({ ...f, [k]: v }))
  }
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(form)
        setForm({ alias: '', url: '', username: '', appPassword: '' })
      }}
      className='grid grid-cols-1 gap-4 md:grid-cols-4'
    >
      <div>
        <Label>Alias</Label>
        <Input value={form.alias} onChange={(e) => set('alias', e.target.value)} placeholder='playpulse | starzbet' required />
      </div>
      <div>
        <Label>URL</Label>
        <Input
          type='url'
          value={form.url}
          onChange={(e) => set('url', e.target.value)}
          placeholder='https://www.playpulse.tech | https://starzbet-durdurulamaz.com'
          required
        />
      </div>
      <div>
        <Label>Username</Label>
        <Input value={form.username} onChange={(e) => set('username', e.target.value)} placeholder='admin' required />
      </div>
      <div>
        <Label>App password</Label>
        <Input type='password' value={form.appPassword} onChange={(e) => set('appPassword', e.target.value)} placeholder='xxxx xxxx xxxx xxxx' required />
      </div>
      <div className='md:col-span-4'>
        <Button type='submit' disabled={busy}>
          {busy ? 'Adding…' : 'Add site'}
        </Button>
      </div>
    </form>
  )
}
