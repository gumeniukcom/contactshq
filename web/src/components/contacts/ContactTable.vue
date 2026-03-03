<template>
  <AppTable
    :columns="columns"
    :rows="contacts"
    :loading="loading"
    empty-text="No contacts found"
  >
    <template #body="{ rows }">
      <tr
        v-for="contact in (rows as Contact[])"
        :key="contact.id"
        class="hover:bg-gray-50 cursor-pointer"
        @click="$emit('select', contact)"
      >
        <!-- Name -->
        <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
          <span v-if="contact.name_prefix" class="text-gray-500 mr-1">{{ contact.name_prefix }}</span>
          {{ contact.first_name }} {{ contact.last_name }}
          <span v-if="contact.nickname" class="ml-1 text-gray-400 text-xs">"{{ contact.nickname }}"</span>
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
          <span v-if="contact.org && contact.title" class="text-gray-300 mx-1">·</span>
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
    </template>
  </AppTable>
</template>

<script setup lang="ts">
import AppTable from '@/components/ui/AppTable.vue'
import type { Contact } from '@/types'

defineProps<{
  contacts: Contact[]
  loading: boolean
}>()

defineEmits<{
  select: [contact: Contact]
  delete: [contact: Contact]
}>()

const columns = [
  { key: 'name', label: 'Name' },
  { key: 'email', label: 'Email' },
  { key: 'phone', label: 'Phone' },
  { key: 'org', label: 'Organization' },
  { key: 'tags', label: 'Tags' },
  { key: 'actions', label: '' },
]

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
