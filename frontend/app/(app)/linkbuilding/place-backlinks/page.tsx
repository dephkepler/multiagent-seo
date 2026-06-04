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
  const [topics, setTopics] = useState('')
  const [provider, setProvider] = useState('')

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

  const run = useMutation({
    mutationFn: () => {
      const topicList = topics
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)
      return api<PlaceBacklinksAccepted>('/linkbuilding/place-backlinks', {
        method: 'POST',
        body: JSON.stringify({
          sheet,
          target_site_url: targetSiteUrl,
          ...(topicList.length ? { topics: topicList } : {}),
          ...(provider ? { provider } : {}),
        }),
      })
    },
    onSuccess: (r) => toast.success(`Queued ${r.sites_queued} donor sites in ${r.sheet}`),
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className='max-w-2xl space-y-6'>
      <Card>
        <h1 className='mb-1 text-lg font-semibold'>Place backlinks on donor sites</h1>
        <p className='mb-6 text-sm text-gray-500'>
          Reads donors where column D=<code className='rounded bg-gray-100 px-1'>yes</code> and H is empty or <code className='rounded bg-gray-100 px-1'>login ok</code>, picks each donor&apos;s latest post via WP REST,
          asks the LLM to weave in a contextual <code className='rounded bg-gray-100 px-1'>{'<a>'}</code> linking to your site, and writes the result into column I.
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
            <Label>Topics filter (optional, comma-separated)</Label>
            <Input value={topics} onChange={(e) => setTopics(e.target.value)} placeholder='education, tech' />
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
    </div>
  )
}
