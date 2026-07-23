export interface ApiErrorShape { code: string; message: string; fields?: Record<string, string> }
export class ApiError extends Error {
  code: string
  fields?: Record<string, string>
  status: number
  constructor(status: number, payload: ApiErrorShape) { super(payload.message); this.name = 'ApiError'; this.status = status; this.code = payload.code; this.fields = payload.fields }
}
let csrfToken = ''
export const setCSRFToken = (token: string) => { csrfToken = token }

async function parse(response: Response) {
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    if (response.status === 401) window.dispatchEvent(new Event('flaredns:auth-expired'))
    throw new ApiError(response.status, payload.error || { code: 'request_failed', message: `请求失败 (${response.status})` })
  }
  return payload
}

export const api = {
  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers)
    if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json')
    if (csrfToken && options.method && !['GET', 'HEAD'].includes(options.method)) headers.set('X-CSRF-Token', csrfToken)
    const payload = await parse(await fetch(`/api${path}`, { ...options, headers, credentials: 'same-origin' }))
    return payload.data as T
  },
  async response<T>(path: string, options: RequestInit = {}): Promise<{ data: T; meta?: Record<string, number> }> {
    const headers = new Headers(options.headers)
    if (csrfToken && options.method && !['GET', 'HEAD'].includes(options.method)) headers.set('X-CSRF-Token', csrfToken)
    return parse(await fetch(`/api${path}`, { ...options, headers, credentials: 'same-origin' }))
  },
}
