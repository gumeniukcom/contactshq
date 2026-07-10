import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import MockAdapter from 'axios-mock-adapter'
import axios from 'axios'
import client, { getApiError } from './client'

describe('getApiError', () => {
  it('prefers the message the API sent', () => {
    const err = { response: { data: { error: 'contact not found' } } }
    expect(getApiError(err)).toBe('contact not found')
  })

  it('falls back to the transport error', () => {
    expect(getApiError(new Error('Network Error'))).toBe('Network Error')
  })

  it('falls back to the supplied default', () => {
    expect(getApiError({}, 'Restore failed')).toBe('Restore failed')
  })
})

describe('401 refresh flow', () => {
  let mock: MockAdapter
  let rawMock: MockAdapter

  beforeEach(() => {
    localStorage.clear()
    mock = new MockAdapter(client)
    rawMock = new MockAdapter(axios)
    // jsdom refuses real navigation; the interceptor assigns to it on refresh failure.
    Object.defineProperty(window, 'location', {
      value: { href: '' },
      writable: true,
    })
  })

  afterEach(() => {
    mock.restore()
    rawMock.restore()
    vi.restoreAllMocks()
  })

  it('attaches the stored access token', async () => {
    localStorage.setItem('access_token', 'tok-1')
    mock.onGet('/contacts').reply((config) => {
      expect(config.headers?.Authorization).toBe('Bearer tok-1')
      return [200, { contacts: [] }]
    })

    await client.get('/contacts')
  })

  it('refreshes once and replays the concurrent requests that hit 401', async () => {
    localStorage.setItem('access_token', 'stale')
    localStorage.setItem('refresh_token', 'refresh-1')

    let refreshCalls = 0
    rawMock.onPost('/api/v1/auth/refresh').reply(() => {
      refreshCalls++
      return [200, { access_token: 'fresh', refresh_token: 'refresh-2' }]
    })

    let attempt = 0
    mock
      .onGet('/contacts')
      .reply(() => (attempt++ < 2 ? [401, { error: 'invalid token' }] : [200, { ok: true }]))
    mock.onGet('/pipelines').reply(() => [200, { ok: true }])

    const [a, b] = await Promise.all([client.get('/contacts'), client.get('/contacts')])

    expect(a.status).toBe(200)
    expect(b.status).toBe(200)
    expect(refreshCalls, 'the refresh endpoint must be called once for concurrent 401s').toBe(1)
    expect(localStorage.getItem('access_token')).toBe('fresh')
    expect(localStorage.getItem('refresh_token')).toBe('refresh-2')
  })

  it('clears the session and redirects when the refresh token is rejected', async () => {
    localStorage.setItem('access_token', 'stale')
    localStorage.setItem('refresh_token', 'expired')

    rawMock.onPost('/api/v1/auth/refresh').reply(401, { error: 'invalid credentials' })
    mock.onGet('/contacts').reply(401, { error: 'invalid token' })

    await expect(client.get('/contacts')).rejects.toBeTruthy()

    expect(localStorage.getItem('access_token')).toBeNull()
    expect(localStorage.getItem('refresh_token')).toBeNull()
    expect(window.location.href).toContain('/app/login')
  })

  it('redirects immediately when there is no refresh token at all', async () => {
    localStorage.setItem('access_token', 'stale')
    mock.onGet('/contacts').reply(401, { error: 'invalid token' })

    await expect(client.get('/contacts')).rejects.toBeTruthy()

    expect(window.location.href).toContain('/app/login')
  })

  it('replays once and gives up, rather than looping on an endpoint that always 401s', async () => {
    localStorage.setItem('access_token', 'stale')
    localStorage.setItem('refresh_token', 'refresh-1')

    let refreshCalls = 0
    rawMock.onPost('/api/v1/auth/refresh').reply(() => {
      refreshCalls++
      return [200, { access_token: 'fresh', refresh_token: 'refresh-2' }]
    })

    let calls = 0
    mock.onGet('/contacts').reply(() => {
      calls++
      return [401, { error: 'still unauthorized' }]
    })

    let rejected = false
    await client.get('/contacts').catch(() => {
      rejected = true
    })

    expect(rejected).toBe(true)
    expect(calls, 'one original attempt plus exactly one replay').toBe(2)
    expect(refreshCalls, 'the token is refreshed once, not once per attempt').toBe(1)
  })
})
