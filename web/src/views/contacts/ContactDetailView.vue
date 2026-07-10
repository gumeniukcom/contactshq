<template>
  <div class="max-w-2xl">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-foreground">Edit Contact</h1>
    </div>

    <AppCard>
      <div v-if="loadingContact" class="py-8 text-center text-muted-foreground">Loading...</div>
      <ContactForm
        v-else
        ref="formRef"
        :initial="contact"
        submit-label="Update"
        :loading="saving"
        @submit="handleUpdate"
        @cancel="router.back()"
      />
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getContact } from '@/api/contacts'
import { useContactsStore } from '@/stores/contacts'
import type { Contact } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import ContactForm from '@/components/contacts/ContactForm.vue'
import { useToast } from '@/composables/useToast'
import { getApiError } from '@/api/client'

const toast = useToast()

const route = useRoute()
const router = useRouter()
const store = useContactsStore()

const contact = ref<Partial<Contact>>({})
const formRef = ref<InstanceType<typeof ContactForm>>()
const loadingContact = ref(true)
const saving = ref(false)
const id = route.params.id as string

onMounted(async () => {
  try {
    const { data } = await getContact(id)
    contact.value = data
  } finally {
    loadingContact.value = false
  }
})

async function handleUpdate() {
  if (!formRef.value) return
  saving.value = true
  try {
    await store.updateContact(id, formRef.value.getFieldsPayload())
    toast.success('Contact saved')
    router.push({ name: 'contact-detail', params: { id } })
  } catch (err: unknown) {
    toast.error(getApiError(err, 'Failed to save contact'))
  } finally {
    saving.value = false
  }
}
</script>
