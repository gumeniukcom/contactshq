import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { jwtDecode } from 'jwt-decode'
import { login as apiLogin } from '@/api/auth'
import { getMe } from '@/api/users'
import type { User, LoginInput } from '@/types'

interface JwtPayload {
  sub: string
  role: string
  exp: number
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string | null>(localStorage.getItem('access_token'))

  const isAuthenticated = computed(() => !!token.value)

  const isAdmin = computed(() => {
    if (!token.value) return false
    try {
      const decoded = jwtDecode<JwtPayload>(token.value)
      return decoded.role === 'admin'
    } catch {
      return false
    }
  })

  async function login(input: LoginInput) {
    const { data } = await apiLogin(input)
    token.value = data.access_token
    localStorage.setItem('access_token', data.access_token)
    localStorage.setItem('refresh_token', data.refresh_token)
    await fetchUser()
  }

  async function fetchUser() {
    try {
      const { data } = await getMe()
      user.value = data
    } catch {
      logout()
    }
  }

  function logout() {
    user.value = null
    token.value = null
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
  }

  return { user, token, isAuthenticated, isAdmin, login, fetchUser, logout }
})
