<template>
  <div class="max-w-lg">
    <h1 class="text-2xl font-bold text-foreground mb-6">Profile</h1>

    <AppCard>
      <form class="space-y-4" @submit.prevent="handleSave">
        <AppInput v-model="form.display_name" label="Display Name" id="display_name" />
        <AppInput v-model="form.email" label="Email" type="email" id="email" />

        <div class="flex justify-end gap-3 pt-4">
          <AppButton type="submit" :loading="loading">Save Changes</AppButton>
        </div>

        <p v-if="success" class="text-sm text-green-600 dark:text-green-400">Profile updated successfully.</p>
        <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
      </form>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { updateProfile } from '@/api/users'
import AppCard from '@/components/ui/AppCard.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'

const auth = useAuthStore()
const loading = ref(false)
const success = ref(false)
const error = ref('')

const form = reactive({
  display_name: '',
  email: '',
})

onMounted(() => {
  if (auth.user) {
    form.display_name = auth.user.display_name
    form.email = auth.user.email
  }
})

async function handleSave() {
  loading.value = true
  success.value = false
  error.value = ''
  try {
    await updateProfile(form)
    await auth.fetchUser()
    success.value = true
  } catch (e: unknown) {
    error.value = (e as { response?: { data?: { error?: string } } })?.response?.data?.error || 'Failed to update'
  } finally {
    loading.value = false
  }
}
</script>
