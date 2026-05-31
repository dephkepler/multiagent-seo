import { clearToken, getToken } from './auth'

const BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8889'

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

export async function api<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken()
  const headers = new Headers(init.headers)
  if (!headers.has('Content-Type') && init.body) headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`${BASE}${path}`, { ...init, headers })
  const text = await res.text()
  const body = text ? safeParse(text) : null

  if (res.status === 401) {
    clearToken()
    throw new ApiError(401, 'unauthorized')
  }
  if (!res.ok) {
    // RFC 7807 problem+json uses `title`/`detail`; fall back to body or status.
    const msg = body?.title || body?.detail || (typeof body === 'string' ? body : res.statusText)
    throw new ApiError(res.status, msg)
  }
  return body as T
}

function safeParse(s: string): any {
  try {
    return JSON.parse(s)
  } catch {
    return s
  }
}
