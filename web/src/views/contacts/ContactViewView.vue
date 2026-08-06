<template>
  <div class="max-w-3xl">
    <div v-if="loading" class="py-12 text-center text-sm text-muted-foreground">Loading…</div>

    <template v-else-if="contact">
      <!-- Header -->
      <div class="flex flex-wrap items-start justify-between gap-4 mb-6">
        <div class="flex items-center gap-4 min-w-0">
          <ContactAvatar
            :first-name="contact.first_name"
            :last-name="contact.last_name"
            :photo-uri="contact.photo_uri"
            size="lg"
          />
          <div class="min-w-0">
            <h1 class="text-2xl font-bold text-foreground truncate">{{ displayName }}</h1>
            <p v-if="subtitle" class="text-sm text-muted-foreground truncate">{{ subtitle }}</p>
            <div v-if="categories.length" class="mt-2 flex flex-wrap gap-1">
              <span
                v-for="c in categories"
                :key="c"
                class="inline-flex items-center rounded-full bg-muted px-2 py-0.5 text-xs text-foreground"
              >
                {{ c }}
              </span>
            </div>
          </div>
        </div>
        <div class="flex flex-wrap gap-2">
          <AppButton variant="secondary" size="sm" @click="showQR = true">QR Code</AppButton>
          <AppButton variant="secondary" size="sm" @click="downloadVCard">Download .vcf</AppButton>
          <AppButton size="sm" @click="router.push({ name: 'contact-edit', params: { id } })">Edit</AppButton>
        </div>
      </div>

      <!-- Only sections that hold something are rendered: an empty "No addresses" block is
           noise on the screen people reach for to look up a phone number. -->
      <AppCard v-if="emails.length || phones.length" class="mb-4">
        <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Contact</h2>
        <dl class="space-y-2">
          <CopyableRow
            v-for="e in emails"
            :key="e.id"
            :label="e.type || 'email'"
            :value="e.value"
            :href="`mailto:${e.value}`"
          />
          <CopyableRow
            v-for="p in phones"
            :key="p.id"
            :label="p.type || 'phone'"
            :value="p.value"
            :href="`tel:${p.value}`"
          />
        </dl>
      </AppCard>

      <AppCard v-if="contact.org || contact.department || contact.title || contact.role" class="mb-4">
        <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">
          Organization
        </h2>
        <dl class="space-y-2">
          <CopyableRow v-if="contact.org" label="Company" :value="contact.org" />
          <CopyableRow v-if="contact.department" label="Department" :value="contact.department" />
          <CopyableRow v-if="contact.title" label="Title" :value="contact.title" />
          <CopyableRow v-if="contact.role" label="Role" :value="contact.role" />
        </dl>
      </AppCard>

      <AppCard v-if="addresses.length" class="mb-4">
        <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Addresses</h2>
        <dl class="space-y-2">
          <CopyableRow
            v-for="a in addresses"
            :key="a.id"
            :label="a.type || 'address'"
            :value="formatAddress(a)"
          />
        </dl>
      </AppCard>

      <AppCard v-if="urls.length || ims.length" class="mb-4">
        <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">
          Web & Messaging
        </h2>
        <dl class="space-y-2">
          <CopyableRow
            v-for="u in urls"
            :key="u.id"
            :label="u.type || 'url'"
            :value="u.value"
            :href="u.value"
          />
          <CopyableRow v-for="im in ims" :key="im.id" :label="im.type || 'im'" :value="im.value" />
        </dl>
      </AppCard>

      <AppCard v-if="contact.bday || contact.anniversary || contact.nickname || contact.note" class="mb-4">
        <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Personal</h2>
        <dl class="space-y-2">
          <CopyableRow v-if="contact.nickname" label="Nickname" :value="contact.nickname" />
          <CopyableRow v-if="contact.bday" label="Birthday" :value="contact.bday" />
          <CopyableRow v-if="contact.anniversary" label="Anniversary" :value="contact.anniversary" />
          <CopyableRow v-if="contact.note" label="Note" :value="contact.note" />
        </dl>
      </AppCard>

      <div class="flex justify-between pt-2">
        <button class="text-sm text-accent hover:underline" @click="router.push({ name: 'contacts' })">
          ← Back to Contacts
        </button>
        <AppButton variant="danger" size="sm" @click="showDeleteConfirm = true">Delete</AppButton>
      </div>

      <AppModal :show="showQR" label="Contact QR code" @close="showQR = false">
        <h3 class="text-lg font-medium text-foreground mb-4">{{ displayName }}</h3>
        <img v-if="qrUrl" :src="qrUrl" :alt="`QR code for ${displayName}`" class="mx-auto" />
        <p v-else class="text-center text-sm text-muted-foreground py-8">Loading QR code…</p>
      </AppModal>

      <ConfirmDialog
        :show="showDeleteConfirm"
        title="Delete Contact"
        :message="`Delete ${displayName}? This cannot be undone.`"
        confirm-text="Delete"
        :loading="deleting"
        @confirm="handleDelete"
        @cancel="showDeleteConfirm = false"
      />
    </template>

    <div v-else class="py-12 text-center text-sm text-muted-foreground">Contact not found.</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Contact, ContactAddress } from '@/types'
import { getContact, getVCard, getQRCode } from '@/api/contacts'
import { useContactsStore } from '@/stores/contacts'
import { useToast } from '@/composables/useToast'
import { getApiError } from '@/api/client'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppModal from '@/components/ui/AppModal.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import ContactAvatar from '@/components/contacts/ContactAvatar.vue'
import CopyableRow from '@/components/contacts/CopyableRow.vue'

const route = useRoute()
const router = useRouter()
const store = useContactsStore()
const toast = useToast()

const id = route.params.id as string
const contact = ref<Contact | null>(null)
const loading = ref(true)
const deleting = ref(false)
const showQR = ref(false)
const showDeleteConfirm = ref(false)
const qrUrl = ref('')

const emails = computed(() => contact.value?.emails ?? [])
const phones = computed(() => contact.value?.phones ?? [])
const addresses = computed(() => contact.value?.addresses ?? [])
const urls = computed(() => contact.value?.urls ?? [])
const ims = computed(() => contact.value?.ims ?? [])
const categories = computed(() => (contact.value?.categories ?? []).map((c) => c.value))

const displayName = computed(() => {
  const c = contact.value
  if (!c) return ''
  return [c.name_prefix, c.first_name, c.last_name].filter(Boolean).join(' ') || c.email || 'Unnamed contact'
})

const subtitle = computed(() => {
  const c = contact.value
  if (!c) return ''
  return [c.title, c.org].filter(Boolean).join(' · ')
})

function formatAddress(a: ContactAddress): string {
  return [a.street, a.city, a.region, a.postal_code, a.country].filter(Boolean).join(', ')
}

onMounted(async () => {
  try {
    const { data } = await getContact(id)
    contact.value = data
  } catch (err: unknown) {
    toast.error(getApiError(err, 'Failed to load contact'))
  } finally {
    loading.value = false
  }
})

// The QR code is generated server-side; fetch it only when the user asks to see it.
watch(showQR, async (open) => {
  if (!open || qrUrl.value) return
  try {
    const { data } = await getQRCode(id)
    qrUrl.value = URL.createObjectURL(new Blob([data], { type: 'image/png' }))
  } catch (err: unknown) {
    toast.error(getApiError(err, 'Failed to load QR code'))
  }
})

onUnmounted(() => {
  if (qrUrl.value) URL.revokeObjectURL(qrUrl.value)
})

async function downloadVCard() {
  try {
    const { data } = await getVCard(id)
    const url = URL.createObjectURL(new Blob([data], { type: 'text/vcard' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${contact.value?.first_name || 'contact'}.vcf`
    a.click()
    URL.revokeObjectURL(url)
  } catch (err: unknown) {
    toast.error(getApiError(err, 'Failed to download vCard'))
  }
}

async function handleDelete() {
  deleting.value = true
  try {
    await store.deleteContact(id)
    toast.success('Contact deleted')
    router.push({ name: 'contacts' })
  } catch (err: unknown) {
    toast.error(getApiError(err, 'Failed to delete contact'))
  } finally {
    deleting.value = false
  }
}
</script>
