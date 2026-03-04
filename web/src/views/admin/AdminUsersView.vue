<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-foreground">Users</h1>
      <RouterLink to="/admin/users/new">
        <AppButton>Create User</AppButton>
      </RouterLink>
    </div>

    <AppCard>
      <AppTable :columns="columns" :rows="users" :loading="loading" empty-text="No users found">
        <template #body="{ rows }">
          <tr v-for="u in (rows as User[])" :key="u.id" class="hover:bg-muted/50">
            <td class="px-4 py-4 text-sm font-medium text-foreground">{{ u.email }}</td>
            <td class="px-4 py-4 text-sm text-muted-foreground">{{ u.display_name }}</td>
            <td class="px-4 py-4 text-sm">
              <select
                :value="u.role"
                class="rounded-md border-input bg-background text-foreground text-sm px-2 py-1 border"
                @change="handleRoleChange(u.id, ($event.target as HTMLSelectElement).value)"
              >
                <option value="user">User</option>
                <option value="admin">Admin</option>
              </select>
            </td>
            <td class="px-4 py-4 text-sm text-muted-foreground">{{ formatDate(u.created_at) }}</td>
            <td class="px-4 py-4 text-sm text-right">
              <button class="text-destructive hover:text-destructive/80" @click="confirmDelete(u)">Delete</button>
            </td>
          </tr>
        </template>
      </AppTable>
    </AppCard>

    <ConfirmDialog
      :show="!!deleteTarget"
      title="Delete User"
      :message="`Are you sure you want to delete ${deleteTarget?.email}?`"
      confirm-text="Delete"
      @confirm="handleDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { listUsers, updateUserRole, deleteUser } from '@/api/admin'
import type { User } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppTable from '@/components/ui/AppTable.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import { formatDate } from '@/utils/date'

const users = ref<User[]>([])
const loading = ref(true)
const deleteTarget = ref<User | null>(null)

const columns = [
  { key: 'email', label: 'Email' },
  { key: 'display_name', label: 'Name' },
  { key: 'role', label: 'Role' },
  { key: 'created_at', label: 'Created' },
  { key: 'actions', label: '' },
]

async function load() {
  loading.value = true
  try {
    const { data } = await listUsers()
    users.value = data.users
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function handleRoleChange(id: string, role: string) {
  await updateUserRole(id, { role })
  await load()
}

function confirmDelete(u: User) {
  deleteTarget.value = u
}

async function handleDelete() {
  if (!deleteTarget.value) return
  await deleteUser(deleteTarget.value.id)
  deleteTarget.value = null
  await load()
}

</script>
