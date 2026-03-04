<template>
  <div class="overflow-x-auto">
    <table class="min-w-full divide-y divide-gray-200">
      <thead class="bg-gray-50 sticky top-0 z-10">
        <tr>
          <!-- Checkbox column -->
          <th v-if="showCheckboxes" class="w-12 px-4 py-3">
            <input
              type="checkbox"
              :checked="allSelected"
              :indeterminate="someSelected && !allSelected"
              class="rounded border-gray-300 text-indigo-600"
              @change="$emit('selectAll')"
            />
          </th>
          <SortableHeader label="Name" column-key="name" :current-sort="sortBy" :current-dir="sortDir" @sort="$emit('sort', $event)" />
          <SortableHeader label="Email" column-key="email" :current-sort="sortBy" :current-dir="sortDir" @sort="$emit('sort', $event)" />
          <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Phone</th>
          <SortableHeader label="Organization" column-key="org" :current-sort="sortBy" :current-dir="sortDir" @sort="$emit('sort', $event)" />
          <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Tags</th>
          <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider"></th>
        </tr>
      </thead>
      <tbody class="bg-white divide-y divide-gray-200">
        <!-- Loading skeleton -->
        <template v-if="loading">
          <tr v-for="i in 5" :key="'skeleton-' + i" class="animate-pulse">
            <td v-if="showCheckboxes" class="px-4 py-4"><div class="h-4 w-4 bg-gray-200 rounded" /></td>
            <td class="px-6 py-4"><div class="flex items-center gap-3"><div class="h-8 w-8 bg-gray-200 rounded-full" /><div class="h-4 w-28 bg-gray-200 rounded" /></div></td>
            <td class="px-6 py-4"><div class="h-4 w-36 bg-gray-200 rounded" /></td>
            <td class="px-6 py-4"><div class="h-4 w-24 bg-gray-200 rounded" /></td>
            <td class="px-6 py-4"><div class="h-4 w-24 bg-gray-200 rounded" /></td>
            <td class="px-6 py-4"><div class="h-4 w-16 bg-gray-200 rounded" /></td>
            <td class="px-6 py-4"></td>
          </tr>
        </template>

        <!-- Empty state -->
        <tr v-else-if="contacts.length === 0">
          <td :colspan="showCheckboxes ? 7 : 6" class="px-6 py-12 text-center text-sm text-gray-500">
            No contacts found
          </td>
        </tr>

        <!-- Data rows -->
        <tr
          v-else
          v-for="contact in contacts"
          :key="contact.id"
          class="hover:bg-gray-50 cursor-pointer"
          @click="$emit('select', contact)"
        >
          <!-- Checkbox -->
          <td v-if="showCheckboxes" class="w-12 px-4 py-4">
            <input
              type="checkbox"
              :checked="ids.has(contact.id)"
              class="rounded border-gray-300 text-indigo-600"
              @click.stop
              @change="$emit('toggleSelect', contact.id)"
            />
          </td>

          <!-- Name with avatar -->
          <td class="px-6 py-4 whitespace-nowrap">
            <div class="flex items-center gap-3">
              <ContactAvatar
                :first-name="contact.first_name"
                :last-name="contact.last_name"
                :photo-uri="contact.photo_uri"
                size="sm"
              />
              <div class="min-w-0">
                <p class="text-sm font-medium text-gray-900 truncate">
                  <span v-if="contact.name_prefix" class="text-gray-500 font-normal">{{ contact.name_prefix }} </span>
                  {{ displayName(contact) }}
                </p>
                <p v-if="contact.nickname" class="text-xs text-gray-400 truncate">"{{ contact.nickname }}"</p>
              </div>
            </div>
          </td>

          <!-- Email with +N badge -->
          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
            <span>{{ primaryEmail(contact) }}</span>
            <span
              v-if="extraEmailCount(contact) > 0"
              class="ml-1.5 inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-700"
            >
              +{{ extraEmailCount(contact) }}
            </span>
          </td>

          <!-- Phone with +N badge -->
          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
            <span>{{ primaryPhone(contact) }}</span>
            <span
              v-if="extraPhoneCount(contact) > 0"
              class="ml-1.5 inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-700"
            >
              +{{ extraPhoneCount(contact) }}
            </span>
          </td>

          <!-- Org + title -->
          <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
            {{ contact.org }}
            <span v-if="contact.org && contact.title" class="text-gray-300 mx-1">&middot;</span>
            <span v-if="contact.title" class="text-gray-400">{{ contact.title }}</span>
          </td>

          <!-- Categories -->
          <td class="px-6 py-4 text-sm text-gray-500">
            <span
              v-for="cat in (contact.categories ?? []).slice(0, 3)"
              :key="cat.id"
              class="inline-flex items-center mr-1 px-2 py-0.5 rounded-full text-xs bg-gray-100 text-gray-600"
            >
              {{ cat.value }}
            </span>
            <span
              v-if="(contact.categories?.length ?? 0) > 3"
              class="text-xs text-gray-400"
            >
              +{{ (contact.categories?.length ?? 0) - 3 }}
            </span>
          </td>

          <!-- Actions -->
          <td class="px-6 py-4 whitespace-nowrap text-right text-sm">
            <button
              class="text-red-600 hover:text-red-800 ml-3"
              @click.stop="$emit('delete', contact)"
            >
              Delete
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Contact } from '@/types'
import ContactAvatar from './ContactAvatar.vue'
import SortableHeader from '@/components/ui/SortableHeader.vue'

const props = withDefaults(defineProps<{
  contacts: Contact[]
  loading: boolean
  sortBy?: string
  sortDir?: string
  selectedIds?: Set<string>
  showCheckboxes?: boolean
}>(), {
  sortBy: 'name',
  sortDir: 'asc',
  showCheckboxes: false,
})

defineEmits<{
  select: [contact: Contact]
  delete: [contact: Contact]
  sort: [key: string]
  toggleSelect: [id: string]
  selectAll: []
}>()

const ids = computed(() => props.selectedIds ?? new Set<string>())

const allSelected = computed(() =>
  props.contacts.length > 0 && props.contacts.every(c => ids.value.has(c.id))
)

const someSelected = computed(() =>
  props.contacts.some(c => ids.value.has(c.id))
)

function displayName(c: Contact): string {
  const first = c.first_name || ''
  const last = c.last_name || ''
  const name = `${first} ${last}`.trim()
  if (name) return name
  if (c.email) return c.email
  if (c.emails?.length) return c.emails[0].value
  if (c.org) return c.org
  return 'Unnamed'
}

function primaryEmail(c: Contact): string {
  if (c.emails && c.emails.length > 0) {
    const pref = c.emails.find((e) => e.pref === 1) ?? c.emails[0]
    return pref.value
  }
  return c.email
}

function primaryPhone(c: Contact): string {
  if (c.phones && c.phones.length > 0) {
    const pref = c.phones.find((p) => p.pref === 1) ?? c.phones[0]
    return pref.value
  }
  return c.phone
}

function extraEmailCount(c: Contact): number {
  if (c.emails && c.emails.length > 1) return c.emails.length - 1
  return 0
}

function extraPhoneCount(c: Contact): number {
  if (c.phones && c.phones.length > 1) return c.phones.length - 1
  return 0
}
</script>
