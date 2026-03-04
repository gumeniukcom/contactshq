<template>
  <div
    class="bg-white border rounded-lg p-4 hover:shadow-md transition-shadow cursor-pointer"
    :class="selected ? 'ring-2 ring-indigo-500 border-indigo-300' : 'border-gray-200'"
    @click="$emit('select', contact)"
  >
    <div class="flex items-start gap-3">
      <!-- Checkbox -->
      <input
        v-if="showCheckbox"
        type="checkbox"
        :checked="selected"
        class="mt-1 rounded border-gray-300 text-indigo-600"
        @click.stop
        @change="$emit('toggleSelect', contact.id)"
      />

      <ContactAvatar
        :first-name="contact.first_name"
        :last-name="contact.last_name"
        :photo-uri="contact.photo_uri"
        size="lg"
      />
      <div class="min-w-0 flex-1">
        <h3 class="text-sm font-semibold text-gray-900 truncate">
          <span v-if="contact.name_prefix" class="text-gray-500 font-normal">{{ contact.name_prefix }} </span>
          {{ displayName }}
          <span v-if="contact.nickname" class="ml-1 text-gray-400 text-xs font-normal">"{{ contact.nickname }}"</span>
        </h3>
        <p v-if="contact.org" class="text-sm text-gray-500 truncate">
          {{ contact.org }}
          <span v-if="contact.title" class="text-gray-400"> · {{ contact.title }}</span>
        </p>
        <p v-if="primaryEmail" class="text-sm text-gray-500 truncate mt-1">{{ primaryEmail }}</p>
        <p v-if="primaryPhone" class="text-sm text-gray-500 truncate">{{ primaryPhone }}</p>

        <!-- Category chips -->
        <div v-if="contact.categories?.length" class="flex flex-wrap gap-1 mt-2">
          <span
            v-for="cat in contact.categories.slice(0, 3)"
            :key="cat.id"
            class="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-gray-100 text-gray-600"
          >
            {{ cat.value }}
          </span>
          <span
            v-if="contact.categories.length > 3"
            class="text-xs text-gray-400"
          >
            +{{ contact.categories.length - 3 }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Contact } from '@/types'
import ContactAvatar from './ContactAvatar.vue'

const props = defineProps<{
  contact: Contact
  selected?: boolean
  showCheckbox?: boolean
}>()

defineEmits<{
  select: [contact: Contact]
  toggleSelect: [id: string]
}>()

const displayName = computed(() => {
  const first = props.contact.first_name || ''
  const last = props.contact.last_name || ''
  const name = `${first} ${last}`.trim()
  if (name) return name
  // Fallback: show primary email or org
  if (props.contact.email) return props.contact.email
  if (props.contact.emails?.length) return props.contact.emails[0].value
  if (props.contact.org) return props.contact.org
  return 'Unnamed'
})

const primaryEmail = computed(() => {
  if (props.contact.emails?.length) {
    const pref = props.contact.emails.find(e => e.pref === 1) ?? props.contact.emails[0]
    return pref.value
  }
  return props.contact.email
})

const primaryPhone = computed(() => {
  if (props.contact.phones?.length) {
    const pref = props.contact.phones.find(p => p.pref === 1) ?? props.contact.phones[0]
    return pref.value
  }
  return props.contact.phone
})
</script>
