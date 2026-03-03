<template>
  <div class="min-h-screen flex items-center justify-center bg-gray-50 px-4">
    <div class="w-full max-w-md">
      <div class="text-center mb-8">
        <h1 class="text-3xl font-bold text-gray-900">ContactsHQ</h1>
        <p class="mt-2 text-sm text-gray-600">Sign in to your account</p>
      </div>

      <div class="bg-white rounded-lg shadow border border-gray-200 p-8">
        <form class="space-y-4" @submit.prevent="handleLogin">
          <AppInput
            v-model="email"
            label="Email"
            type="email"
            placeholder="you@example.com"
            id="email"
            :error="error"
          />
          <AppInput
            v-model="password"
            label="Password"
            type="password"
            placeholder="Password"
            id="password"
          />
          <AppButton type="submit" class="w-full" :loading="loading">
            Sign In
          </AppButton>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'

const router = useRouter()
const auth = useAuthStore()

const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true
  try {
    await auth.login({ email: email.value, password: password.value })
    router.push({ name: 'dashboard' })
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    error.value = msg || 'Invalid email or password'
  } finally {
    loading.value = false
  }
}
</script>
