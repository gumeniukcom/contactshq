<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-gray-900 mb-6">Import Contacts</h1>

    <div class="flex gap-2 mb-6">
      <button
        v-for="t in tabs"
        :key="t"
        :class="[
          'px-4 py-2 text-sm font-medium rounded-md',
          tab === t ? 'bg-indigo-600 text-white' : 'bg-white text-gray-700 border border-gray-300 hover:bg-gray-50',
        ]"
        @click="tab = t"
      >
        {{ t }}
      </button>
    </div>

    <AppCard>
      <FileUpload :accept="tab === 'vCard' ? '.vcf' : '.csv'" @file="selectedFile = $event" />

      <div class="mt-4 flex justify-end">
        <AppButton :loading="loading" :disabled="!selectedFile" @click="handleImport">
          Import
        </AppButton>
      </div>

      <div v-if="result" class="mt-6 p-4 rounded-md bg-gray-50 border border-gray-200">
        <p class="text-sm">
          <span class="font-medium text-green-700">Imported: {{ result.imported }}</span>
          <span class="mx-2 text-gray-300">|</span>
          <span class="text-gray-600">Skipped: {{ result.skipped }}</span>
        </p>
        <ul v-if="result.errors?.length" class="mt-2 text-sm text-red-600 list-disc list-inside">
          <li v-for="(err, i) in result.errors" :key="i">{{ err }}</li>
        </ul>
      </div>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { importVCard, importCSV } from '@/api/import-export'
import type { ImportResult } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import FileUpload from '@/components/ui/FileUpload.vue'

const tabs = ['vCard', 'CSV'] as const
const tab = ref<(typeof tabs)[number]>('vCard')
const selectedFile = ref<File | null>(null)
const loading = ref(false)
const result = ref<ImportResult | null>(null)

async function handleImport() {
  if (!selectedFile.value) return
  loading.value = true
  result.value = null
  try {
    const fn = tab.value === 'vCard' ? importVCard : importCSV
    const { data } = await fn(selectedFile.value)
    result.value = data
  } catch {
    result.value = { imported: 0, skipped: 0, errors: ['Import failed'] }
  } finally {
    loading.value = false
  }
}
</script>
