import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory('/app/'),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      path: '/',
      component: () => import('@/components/layout/AppLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('@/views/DashboardView.vue'),
        },
        {
          path: 'contacts',
          name: 'contacts',
          component: () => import('@/views/contacts/ContactListView.vue'),
        },
        {
          path: 'contacts/new',
          name: 'contact-create',
          component: () => import('@/views/contacts/ContactCreateView.vue'),
        },
        {
          path: 'contacts/duplicates',
          name: 'duplicates',
          component: () => import('@/views/contacts/DuplicatesView.vue'),
        },
        {
          path: 'contacts/duplicates/:dupId/merge',
          name: 'contact-merge',
          component: () => import('@/views/contacts/ContactMergeView.vue'),
        },
        {
          path: 'contacts/:id',
          name: 'contact-detail',
          component: () => import('@/views/contacts/ContactDetailView.vue'),
        },
        {
          path: 'import',
          name: 'import',
          component: () => import('@/views/import-export/ImportView.vue'),
        },
        {
          path: 'export',
          name: 'export',
          component: () => import('@/views/import-export/ExportView.vue'),
        },
        {
          path: 'pipelines',
          name: 'pipelines',
          component: () => import('@/views/pipelines/PipelineListView.vue'),
        },
        {
          path: 'pipelines/new',
          name: 'pipeline-create',
          component: () => import('@/views/pipelines/PipelineCreateView.vue'),
        },
        {
          path: 'pipelines/:id',
          name: 'pipeline-view',
          component: () => import('@/views/pipelines/PipelineViewView.vue'),
        },
        {
          path: 'pipelines/:id/edit',
          name: 'pipeline-edit',
          component: () => import('@/views/pipelines/PipelineDetailView.vue'),
        },
        {
          path: 'backup',
          name: 'backup',
          component: () => import('@/views/backup/BackupView.vue'),
        },
        {
          path: 'credentials',
          name: 'credentials',
          component: () => import('@/views/credentials/CredentialsView.vue'),
        },
        {
          path: 'sync/conflicts',
          name: 'sync-conflicts',
          component: () => import('@/views/sync/SyncConflictsView.vue'),
        },
        {
          path: 'sync/conflicts/:id',
          name: 'sync-conflict-detail',
          component: () => import('@/views/sync/SyncConflictDetailView.vue'),
        },
        {
          path: 'settings/profile',
          name: 'profile',
          component: () => import('@/views/settings/ProfileView.vue'),
        },
        {
          path: 'settings/password',
          name: 'password',
          component: () => import('@/views/settings/PasswordView.vue'),
        },
        {
          path: 'settings/google',
          name: 'google-settings',
          component: () => import('@/views/settings/GoogleSettingsView.vue'),
        },
        {
          path: 'admin/users',
          name: 'admin-users',
          meta: { requiresAdmin: true },
          component: () => import('@/views/admin/AdminUsersView.vue'),
        },
        {
          path: 'admin/users/new',
          name: 'admin-create-user',
          meta: { requiresAdmin: true },
          component: () => import('@/views/admin/AdminCreateUserView.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login' }
  }

  if (to.meta.requiresAdmin && !auth.isAdmin) {
    return { name: 'dashboard' }
  }

  if (auth.isAuthenticated && !auth.user) {
    await auth.fetchUser()
  }
})

export default router
