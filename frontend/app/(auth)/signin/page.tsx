'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { setSession } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input, Label } from '@/components/ui/input'

export default function SignInPage() {
  const router = useRouter()
  const [email, setEmail] = useState('verify@local.test')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const res = await api<{ token: string; role: string }>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      })
      setSession(res.token, res.role)
      // An advocate has no business on /generate — that page, and most of the
      // menu, is not theirs.
      router.push(res.role === 'advocate' ? '/my' : '/generate')
    } catch (e) {
      const message = (e as Error).message
      setError(message)
      toast.error(message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className='flex min-h-screen flex-col items-center justify-center gap-6 p-4 sm:p-6'>
      <p className='text-sm font-semibold tracking-tight text-emerald-600'>multiagent-seo</p>
      <Card className='w-full max-w-sm'>
        <div className='mb-6'>
          <h1 className='text-xl font-semibold text-gray-900'>Sign in</h1>
          <p className='mt-1 text-sm text-gray-500'>Sign in with your account credentials to continue.</p>
        </div>
        {error && <div className='mb-4 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700'>{error}</div>}
        <form onSubmit={submit} className='space-y-4'>
          <div>
            <Label htmlFor='email'>Email</Label>
            <Input
              id='email'
              type='email'
              autoComplete='email'
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
          <div>
            <Label htmlFor='password'>Password</Label>
            <Input
              id='password'
              type='password'
              autoComplete='current-password'
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          <Button type='submit' disabled={busy} className='h-11 w-full text-base'>
            {busy ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>
      </Card>
    </div>
  )
}
