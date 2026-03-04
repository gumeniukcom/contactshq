<template>
  <form class="space-y-8" @submit.prevent="$emit('submit')">

    <!-- ── Name ─────────────────────────────────────────────── -->
    <section>
      <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Name</h3>
      <div class="grid grid-cols-12 gap-3 mb-3">
        <div class="col-span-2">
          <AppInput v-model="form.name_prefix" label="Prefix" placeholder="Dr." id="name_prefix" />
        </div>
        <div class="col-span-4">
          <AppInput v-model="form.first_name" label="First Name" placeholder="Jane" id="first_name" />
        </div>
        <div class="col-span-2">
          <AppInput v-model="form.middle_name" label="Middle" id="middle_name" />
        </div>
        <div class="col-span-4">
          <AppInput v-model="form.last_name" label="Last Name" placeholder="Doe" id="last_name" />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <AppInput v-model="form.name_suffix" label="Suffix" placeholder="Jr., PhD" id="name_suffix" />
        <AppInput v-model="form.nickname" label="Nickname" id="nickname" />
      </div>
    </section>

    <!-- ── Contact ───────────────────────────────────────────── -->
    <section>
      <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Contact</h3>
      <div class="space-y-4">
        <MultiFieldRow
          v-model="form.emails"
          label="Email"
          placeholder="name@example.com"
          input-type="email"
          :type-options="['work', 'home', 'other']"
          add-label="email"
        />
        <MultiFieldRow
          v-model="form.phones"
          label="Phone"
          placeholder="+1 234 567 8900"
          input-type="tel"
          :type-options="['mobile', 'work', 'home', 'fax', 'other']"
          add-label="phone"
        />
      </div>
    </section>

    <!-- ── Organization ──────────────────────────────────────── -->
    <section>
      <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Organization</h3>
      <div class="grid grid-cols-2 gap-3 mb-3">
        <AppInput v-model="form.org" label="Company" placeholder="Acme Inc." id="org" />
        <AppInput v-model="form.department" label="Department" placeholder="Engineering" id="department" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <AppInput v-model="form.title" label="Job Title" placeholder="Software Engineer" id="title" />
        <AppInput v-model="form.role" label="Role" id="role" />
      </div>
    </section>

    <!-- ── Personal ──────────────────────────────────────────── -->
    <section>
      <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Personal</h3>
      <div class="grid grid-cols-2 gap-3 mb-3">
        <AppInput v-model="form.bday" label="Birthday" type="date" id="bday" />
        <AppInput v-model="form.anniversary" label="Anniversary" type="date" id="anniversary" />
      </div>
      <div class="grid grid-cols-2 gap-3 mb-3">
        <div>
          <label for="gender" class="block text-sm font-medium text-foreground mb-1">Gender</label>
          <select
            id="gender"
            v-model="form.gender"
            class="block w-full rounded-md border border-input bg-background text-foreground focus:border-ring focus:ring-ring sm:text-sm px-3 py-2"
          >
            <option value="">—</option>
            <option value="M">Male</option>
            <option value="F">Female</option>
            <option value="N">Neutral / non-binary</option>
            <option value="O">Other</option>
          </select>
        </div>
        <AppInput v-model="form.tz" label="Time Zone" placeholder="America/New_York" id="tz" />
      </div>
      <div>
        <label for="note" class="block text-sm font-medium text-foreground mb-1">Note</label>
        <textarea
          id="note"
          v-model="form.note"
          rows="3"
          class="block w-full rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-ring sm:text-sm px-3 py-2"
          placeholder="Notes..."
        />
      </div>
    </section>

    <!-- ── Addresses ──────────────────────────────────────────── -->
    <section>
      <div class="flex items-center justify-between mb-3">
        <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Addresses</h3>
        <button
          type="button"
          class="text-sm text-accent hover:text-accent/80 font-medium"
          @click="addAddress"
        >
          + Add address
        </button>
      </div>

      <div v-if="form.addresses.length === 0" class="text-sm text-muted-foreground italic">No addresses</div>

      <div
        v-for="(addr, i) in form.addresses"
        :key="i"
        class="border border-border rounded-lg p-4 mb-3 space-y-3 relative"
      >
        <button
          type="button"
          class="absolute top-3 right-3 text-muted-foreground hover:text-destructive transition-colors"
          @click="removeAddress(i)"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-sm font-medium text-foreground mb-1">Type</label>
            <select
              v-model="addr.type"
              class="block w-full rounded-md border border-input bg-background text-foreground focus:border-ring focus:ring-ring sm:text-sm px-3 py-2"
            >
              <option value="">—</option>
              <option value="work">Work</option>
              <option value="home">Home</option>
              <option value="other">Other</option>
            </select>
          </div>
          <AppInput v-model="addr.street" label="Street" :id="`addr_street_${i}`" />
        </div>
        <div class="grid grid-cols-3 gap-3">
          <AppInput v-model="addr.city" label="City" :id="`addr_city_${i}`" />
          <AppInput v-model="addr.region" label="State / Region" :id="`addr_region_${i}`" />
          <AppInput v-model="addr.postal_code" label="Postal Code" :id="`addr_zip_${i}`" />
        </div>
        <AppInput v-model="addr.country" label="Country" :id="`addr_country_${i}`" />
      </div>
    </section>

    <!-- ── Web & Messaging ───────────────────────────────────── -->
    <section>
      <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Web & Messaging</h3>
      <div class="space-y-4">
        <MultiFieldRow
          v-model="form.urls"
          label="Website"
          placeholder="https://example.com"
          input-type="url"
          :type-options="['work', 'home', 'other']"
          add-label="website"
        />
        <MultiFieldRow
          v-model="form.ims"
          label="Instant Messaging"
          placeholder="skype:username"
          add-label="IM"
        />
      </div>
    </section>

    <!-- ── Tags ──────────────────────────────────────────────── -->
    <section>
      <h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-3">Tags</h3>
      <div>
        <label for="categories" class="block text-sm font-medium text-foreground mb-1">
          Categories
          <span class="text-muted-foreground font-normal">(comma-separated)</span>
        </label>
        <input
          id="categories"
          type="text"
          :value="form.categories.join(', ')"
          placeholder="friend, colleague, vip"
          class="block w-full rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-ring sm:text-sm px-3 py-2"
          @input="updateCategories(($event.target as HTMLInputElement).value)"
        />
      </div>
    </section>

    <!-- ── Actions ───────────────────────────────────────────── -->
    <div class="flex justify-end gap-3 pt-4 border-t border-border">
      <AppButton variant="secondary" @click="$emit('cancel')">Cancel</AppButton>
      <AppButton type="submit" :loading="loading">{{ submitLabel }}</AppButton>
    </div>

  </form>
</template>

<script setup lang="ts">
import { reactive, watchEffect } from 'vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import MultiFieldRow from '@/components/contacts/MultiFieldRow.vue'
import type { Contact, ContactFormData } from '@/types'
import { buildVCard, formFromContact, emptyForm } from '@/utils/vcard'

const props = withDefaults(
  defineProps<{
    initial?: Partial<Contact>
    submitLabel?: string
    loading?: boolean
  }>(),
  {
    submitLabel: 'Save',
    loading: false,
  },
)

defineEmits<{
  submit: []
  cancel: []
}>()

const form = reactive<ContactFormData>(emptyForm())

watchEffect(() => {
  if (props.initial) {
    Object.assign(form, formFromContact(props.initial))
  }
})

function addAddress() {
  form.addresses.push({ street: '', city: '', region: '', postal_code: '', country: '', type: '' })
}

function removeAddress(i: number) {
  form.addresses.splice(i, 1)
}

function updateCategories(raw: string) {
  form.categories = raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

/** Returns the payload to send to the API (full vCard as vcard_data). */
function getVCardPayload(): { vcard_data: string } {
  return { vcard_data: buildVCard(form) }
}

defineExpose({ form, getVCardPayload })
</script>
