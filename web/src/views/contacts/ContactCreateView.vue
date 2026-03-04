<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-foreground mb-6">Create Contact</h1>
    <AppCard>
      <ContactForm
        ref="formRef"
        submit-label="Create"
        :loading="loading"
        @submit="handleCreate"
        @cancel="router.back()"
      />
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useContactsStore } from '@/stores/contacts'
import AppCard from '@/components/ui/AppCard.vue'
import ContactForm from '@/components/contacts/ContactForm.vue'

const router = useRouter()
const store = useContactsStore()
const formRef = ref<InstanceType<typeof ContactForm>>()
const loading = ref(false)

async function handleCreate() {
  if (!formRef.value) return
  loading.value = true
  try {
    await store.createContact(formRef.value.getVCardPayload())
    router.push({ name: 'contacts' })
  } finally {
    loading.value = false
  }
}
</script>
