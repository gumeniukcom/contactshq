<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-gray-900">Pipelines</h1>
      <RouterLink to="/pipelines/new">
        <AppButton>Create Pipeline</AppButton>
      </RouterLink>
    </div>

    <AppCard>
      <AppTable :columns="columns" :rows="pipelines" :loading="loading" empty-text="No pipelines yet">
        <template #body="{ rows }">
          <tr
            v-for="p in (rows as Pipeline[])"
            :key="p.id"
            class="hover:bg-gray-50 cursor-pointer"
            @click="router.push({ name: 'pipeline-view', params: { id: p.id } })"
          >
            <td class="px-6 py-4 text-sm font-medium text-gray-900">{{ p.name }}</td>
            <td class="px-6 py-4 text-sm">
              <AppBadge :color="p.enabled ? 'green' : 'gray'">
                {{ p.enabled ? 'Enabled' : 'Disabled' }}
              </AppBadge>
            </td>
            <td class="px-6 py-4 text-sm text-gray-500">{{ p.schedule ? humanizeCron(p.schedule) : '—' }}</td>
            <td class="px-6 py-4 text-sm text-right space-x-2 whitespace-nowrap">
              <AppButton size="sm" variant="secondary" @click.stop="handleTrigger(p.id)">
                Trigger
              </AppButton>
              <AppButton size="sm" variant="secondary" @click.stop="router.push({ name: 'pipeline-edit', params: { id: p.id } })">
                Edit
              </AppButton>
              <AppButton size="sm" variant="danger" @click.stop="confirmDelete(p)">
                Delete
              </AppButton>
            </td>
          </tr>
        </template>
      </AppTable>
    </AppCard>

    <ConfirmDialog
      :show="!!deleteTarget"
      title="Delete Pipeline"
      message="Are you sure you want to delete this pipeline?"
      confirm-text="Delete"
      @confirm="handleDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { listPipelines, deletePipeline, triggerPipeline } from '@/api/pipelines'
import { humanizeCron } from '@/utils/cron'
import type { Pipeline } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppTable from '@/components/ui/AppTable.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'

const router = useRouter()
const pipelines = ref<Pipeline[]>([])
const loading = ref(true)
const deleteTarget = ref<Pipeline | null>(null)

const columns = [
  { key: 'name', label: 'Name' },
  { key: 'enabled', label: 'Status' },
  { key: 'schedule', label: 'Schedule' },
  { key: 'actions', label: '' },
]

async function load() {
  loading.value = true
  try {
    const { data } = await listPipelines()
    pipelines.value = data.pipelines ?? []
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function handleTrigger(id: string) {
  await triggerPipeline(id)
}

function confirmDelete(p: Pipeline) {
  deleteTarget.value = p
}

async function handleDelete() {
  if (!deleteTarget.value) return
  await deletePipeline(deleteTarget.value.id)
  deleteTarget.value = null
  load()
}
</script>
