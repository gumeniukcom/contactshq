<template>
  <div class="max-w-2xl">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-foreground">Edit Contact</h1>
      <div class="flex gap-2">
        <AppButton variant="secondary" @click="downloadVCard">vCard</AppButton>
        <AppButton variant="secondary" @click="showQR = true">QR Code</AppButton>
        <AppButton variant="danger" @click="showDeleteConfirm = true">Delete</AppButton>
      </div>
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

    <AppModal :show="showQR" @close="showQR = false">
      <h3 class="text-lg font-medium text-foreground mb-4">QR Code</h3>
      <div class="flex justify-center">
        <img v-if="qrUrl" :src="qrUrl" alt="QR Code" class="w-64 h-64" />
        <p v-else class="text-muted-foreground">Loading...</p>
      </div>
    </AppModal>

    <ConfirmDialog
      :show="showDeleteConfirm"
      title="Delete Contact"
      message="Are you sure you want to delete this contact?"
      confirm-text="Delete"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getContact, getVCard, getQRCode } from '@/api/contacts'
import { useContactsStore } from '@/stores/contacts'
import type { Contact } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import ContactForm from '@/components/contacts/ContactForm.vue'

const route = useRoute()
const router = useRouter()
const store = useContactsStore()

const contact = ref<Partial<Contact>>({})
const formRef = ref<InstanceType<typeof ContactForm>>()
const loadingContact = ref(true)
const saving = ref(false)
const showQR = ref(false)
const showDeleteConfirm = ref(false)
const qrUrl = ref('')

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
    await store.updateContact(id, formRef.value.getVCardPayload())
    router.push({ name: 'contacts' })
  } finally {
    saving.value = false
  }
}

async function downloadVCard() {
  try {
    const { data } = await getVCard(id)
    const blob = new Blob([data], { type: 'text/vcard' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${contact.value.first_name || 'contact'}.vcf`
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    // ignore
  }
}

onMounted(async () => {
  try {
    const { data } = await getQRCode(id)
    const blob = new Blob([data], { type: 'image/png' })
    qrUrl.value = URL.createObjectURL(blob)
  } catch {
    // ignore
  }
})

async function handleDelete() {
  await store.deleteContact(id)
  router.push({ name: 'contacts' })
}
</script>
