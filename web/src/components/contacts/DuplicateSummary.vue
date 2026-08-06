<template>
  <div class="flex items-start gap-3 min-w-0">
    <ContactAvatar
      :first-name="contact.first_name"
      :last-name="contact.last_name"
      :photo-uri="contact.photo_uri"
      size="sm"
    />
    <div class="space-y-0.5 text-sm min-w-0">
      <p class="font-medium text-foreground truncate">{{ displayName }}</p>

      <p v-if="primaryEmail" class="text-muted-foreground truncate">
        {{ primaryEmail }}
        <span v-if="extraEmails > 0" :class="badgeClass">+{{ extraEmails }}</span>
      </p>

      <p v-if="primaryPhone" class="text-muted-foreground truncate">
        {{ primaryPhone }}
        <span v-if="extraPhones > 0" :class="badgeClass">+{{ extraPhones }}</span>
      </p>

      <p v-if="contact.org" class="text-muted-foreground text-xs truncate">{{ contact.org }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Contact } from '@/types'
import ContactAvatar from './ContactAvatar.vue'

/**
 * A compact one-contact summary for the duplicates list.
 *
 * It replaces a `defineComponent`+`h` block that lived inside DuplicatesView's script: as a
 * render function it could not be tested, styled with the rest of the app, or reused by the
 * merge screen, and it showed only the denormalised email and phone with no hint that a
 * contact might hold more.
 */
const props = defineProps<{ contact: Contact }>()

const badgeClass =
  'ml-1.5 inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium bg-accent/10 text-accent'

const displayName = computed(() => {
  const name = [props.contact.first_name, props.contact.last_name].filter(Boolean).join(' ')
  return name || props.contact.email || '(no name)'
})

// The child collections are only loaded on the single-pair endpoint; in the list the
// denormalised columns are all there is, and a "+N" badge would be a lie there.
const primaryEmail = computed(() => props.contact.emails?.[0]?.value || props.contact.email || '')
const primaryPhone = computed(() => props.contact.phones?.[0]?.value || props.contact.phone || '')

const extraEmails = computed(() => Math.max(0, (props.contact.emails?.length ?? 0) - 1))
const extraPhones = computed(() => Math.max(0, (props.contact.phones?.length ?? 0) - 1))
</script>
