<template>
  <div
    class="bg-card border rounded-lg p-4 hover:shadow-md transition-shadow cursor-pointer"
    :class="selected ? 'ring-2 ring-ring border-accent' : 'border-border'"
    @click="$emit('select', contact)"
  >
    <div class="flex items-start gap-3">
      <!-- Checkbox -->
      <input
        v-if="showCheckbox"
        type="checkbox"
        :checked="selected"
        class="mt-1 rounded border-input text-accent"
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
        <h3 class="text-sm font-semibold text-foreground truncate">
          <span v-if="contact.name_prefix" class="text-muted-foreground font-normal">{{ contact.name_prefix }} </span>
          {{ displayName }}
          <span v-if="contact.nickname" class="ml-1 text-muted-foreground text-xs font-normal">"{{ contact.nickname }}"</span>
        </h3>
        <p v-if="contact.org" class="text-sm text-muted-foreground truncate">
          {{ contact.org }}
          <span v-if="contact.title" class="text-muted-foreground"> · {{ contact.title }}</span>
        </p>
        <p v-if="primaryEmail" class="text-sm text-muted-foreground truncate mt-1">{{ primaryEmail }}</p>
        <p v-if="primaryPhone" class="text-sm text-muted-foreground truncate">{{ primaryPhone }}</p>

        <!-- Category chips -->
        <div v-if="contact.categories?.length" class="flex flex-wrap gap-1 mt-2">
          <span
            v-for="cat in contact.categories.slice(0, 3)"
            :key="cat.id"
            class="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-muted text-muted-foreground"
          >
            {{ cat.value }}
          </span>
          <span
            v-if="contact.categories.length > 3"
            class="text-xs text-muted-foreground"
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
