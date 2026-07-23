import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, setCSRFToken } from './api'

describe('API client', () => {
  beforeEach(() => { vi.restoreAllMocks(); setCSRFToken('csrf-test-token') })

  it('unwraps data and sends CSRF on mutations', async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      const headers = new Headers(init?.headers)
      expect(headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      expect(headers.get('Content-Type')).toBe('application/json')
      return new Response(JSON.stringify({ data: { saved: true } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    vi.stubGlobal('fetch', fetchMock)
    await expect(api.request('/test', { method: 'POST', body: '{}' })).resolves.toEqual({ saved: true })
    expect(fetchMock).toHaveBeenCalledOnce()
  })
})
