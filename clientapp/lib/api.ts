import { getInitData } from './telegram'

// Same origin in production: the proxy serves this bundle at /app and the Go
// listener under /api. So there is no API URL baked into a static build, no
// CORS preflight doubling every request on a phone, and no stale port.
const BASE = process.env.NEXT_PUBLIC_API_BASE || '/api'

export class ApiError extends Error {
  constructor(
    public status: number,
    public detail: string
  ) {
    super(detail)
  }
}

/**
 * The launch itself is the credential, so a 401 is not "log in" — there is no
 * login. Separated from other errors so the UI can say "reopen the app", which
 * is the only thing that actually fixes it, and so react-query never retries it.
 */
export class StaleLaunchError extends ApiError {
  constructor(detail = 'stale launch') {
    super(401, detail)
  }
}

/** A 403 on /client/me means the CRM has never seen this person — not a fault. */
export class NotAClientError extends ApiError {
  constructor(detail = 'not a client yet') {
    super(403, detail)
  }
}

/** The hour was taken between the grid being drawn and this request landing. */
export class SlotTakenError extends ApiError {
  constructor(detail = 'slot taken') {
    super(409, detail)
  }
}

interface ProblemBody {
  detail?: string
  title?: string
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const initData = getInitData()
  if (!initData) throw new StaleLaunchError('no init data')

  const headers = new Headers(init.headers)
  // Telegram's own scheme for handing raw init data to a backend. Not a bearer
  // token: nothing here was ever issued to us. It travels in a header rather
  // than a query so it stays out of access logs, history and Referer.
  headers.set('Authorization', `tma ${initData}`)
  if (init.body) headers.set('Content-Type', 'application/json')

  const res = await fetch(`${BASE}${path}`, { ...init, headers })
  const text = await res.text()
  const body: ProblemBody | null = text ? safeParse<ProblemBody>(text) : null
  const detail = body?.detail || body?.title || res.statusText

  if (res.status === 401) throw new StaleLaunchError(detail)
  if (res.status === 403) throw new NotAClientError(detail)
  if (res.status === 409) throw new SlotTakenError(detail)
  if (!res.ok) throw new ApiError(res.status, detail)
  return (text ? JSON.parse(text) : null) as T
}

function safeParse<T>(s: string): T | null {
  try {
    return JSON.parse(s) as T
  } catch {
    return null
  }
}
