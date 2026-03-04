<template>
  <div class="max-w-lg">
    <h1 class="text-2xl font-bold text-foreground mb-6">Create User</h1>

    <AppCard>
      <form class="space-y-4" @submit.prevent="handleCreate">
        <AppInput v-model="form.email" label="Email" type="email" id="email" />
        <AppInput v-model="form.display_name" label="Display Name" id="display_name" />
        <AppInput v-model="form.password" label="Password" type="password" id="password" />

        <div class="flex justify-end gap-3 pt-4">
          <AppButton variant="secondary" @click="router.back()">Cancel</AppButton>
          <AppButton type="submit" :loading="loading">Create User</AppButton>
        </div>

        <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
      </form>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { createUser } from '@/api/admin'
import AppCard from '@/components/ui/AppCard.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'

const router = useRouter()
const loading = ref(false)
const error = ref('')

const form = reactive({
  email: '',
  display_name: '',
  password: '',
})

async function handleCreate() {
  loading.value = true
  error.value = ''
  try {
    await createUser(form)
    router.push({ name: 'admin-users' })
  } catch (e: unknown) {
    error.value = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to create user'
  } finally {
    loading.value = false
  }
}
</script>
