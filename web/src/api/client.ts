import axios from 'axios'
import type { TokenPair } from '@/types'

const client = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
})

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let isRefreshing = false
let failedQueue: Array<{
  resolve: (value: unknown) => void
  reject: (reason: unknown) => void
}> = []

function endSession() {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
  window.location.href = '/app/login'
}

function processQueue(error: unknown) {
  failedQueue.forEach((p) => {
    if (error) {
      p.reject(error)
    } else {
      p.resolve(undefined)
    }
  })
  failedQueue = []
}

client.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config

    if (error.response?.status === 401 && !originalRequest._retry) {
      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject })
        }).then(() => client(originalRequest))
      }

      const refreshToken = localStorage.getItem('refresh_token')
      if (!refreshToken) {
        // Do not raise the isRefreshing flag here: nothing is going to lower it, and
        // every later 401 would queue behind a refresh that never runs.
        endSession()
        return Promise.reject(error)
      }

      originalRequest._retry = true
      isRefreshing = true

      try {
        const { data } = await axios.post<TokenPair>('/api/v1/auth/refresh', {
          refresh_token: refreshToken,
        })
        localStorage.setItem('access_token', data.access_token)
        localStorage.setItem('refresh_token', data.refresh_token)
        processQueue(null)
        return client(originalRequest)
      } catch (refreshError) {
        processQueue(refreshError)
        endSession()
        return Promise.reject(refreshError)
      } finally {
        isRefreshing = false
      }
    }

    return Promise.reject(error)
  },
)

/**
 * Pull a human-readable message out of an API failure. The server answers errors as
 * `{"error": "..."}`; anything else falls back to the transport error.
 */
export function getApiError(e: unknown, fallback = 'Something went wrong'): string {
  const response = (e as { response?: { data?: { error?: string } } })?.response
  if (response?.data?.error) return response.data.error
  const message = (e as Error)?.message
  return message || fallback
}

export default client
