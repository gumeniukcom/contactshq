<template>
  <div class="max-w-lg">
    <h1 class="text-2xl font-bold text-foreground mb-6">Change Password</h1>

    <AppCard>
      <form class="space-y-4" @submit.prevent="handleChange">
        <AppInput v-model="form.old_password" label="Current Password" type="password" id="old_password" />
        <AppInput v-model="form.new_password" label="New Password" type="password" id="new_password" />
        <AppInput v-model="confirmPassword" label="Confirm New Password" type="password" id="confirm_password"
          :error="confirmError" />

        <div class="flex justify-end gap-3 pt-4">
          <AppButton type="submit" :loading="loading">Change Password</AppButton>
        </div>

        <p v-if="success" class="text-sm text-green-600 dark:text-green-400">Password changed successfully.</p>
        <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
      </form>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { changePassword } from '@/api/users'
import AppCard from '@/components/ui/AppCard.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'

const loading = ref(false)
const success = ref(false)
const error = ref('')
const confirmPassword = ref('')

const form = reactive({
  old_password: '',
  new_password: '',
})

const confirmError = computed(() =>
  confirmPassword.value && confirmPassword.value !== form.new_password
    ? 'Passwords do not match'
    : '',
)

async function handleChange() {
  if (form.new_password !== confirmPassword.value) return
  loading.value = true
  success.value = false
  error.value = ''
  try {
    await changePassword(form)
    success.value = true
    form.old_password = ''
    form.new_password = ''
    confirmPassword.value = ''
  } catch (e: unknown) {
    error.value = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to change password'
  } finally {
    loading.value = false
  }
}
</script>
