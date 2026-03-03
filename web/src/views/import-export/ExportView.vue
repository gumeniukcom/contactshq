<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-gray-900 mb-6">Export Contacts</h1>

    <AppCard>
      <p class="text-sm text-gray-600 mb-6">Download all your contacts in one of the available formats.</p>
      <div class="flex flex-wrap gap-3">
        <AppButton @click="handleExport('vcard')" :loading="loading === 'vcard'">
          Download vCard
        </AppButton>
        <AppButton variant="secondary" @click="handleExport('csv')" :loading="loading === 'csv'">
          Download CSV
        </AppButton>
        <AppButton variant="secondary" @click="handleExport('json')" :loading="loading === 'json'">
          Download JSON
        </AppButton>
      </div>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { exportVCard, exportCSV, exportJSON } from '@/api/import-export'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'

const loading = ref<string | null>(null)

const exportFns = {
  vcard: { fn: exportVCard, ext: 'vcf', mime: 'text/vcard' },
  csv: { fn: exportCSV, ext: 'csv', mime: 'text/csv' },
  json: { fn: exportJSON, ext: 'json', mime: 'application/json' },
}

async function handleExport(format: keyof typeof exportFns) {
  loading.value = format
  try {
    const { fn, ext } = exportFns[format]
    const { data } = await fn()
    const blob = data instanceof Blob ? data : new Blob([data])
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `contacts.${ext}`
    a.click()
    URL.revokeObjectURL(url)
  } finally {
    loading.value = null
  }
}
</script>
