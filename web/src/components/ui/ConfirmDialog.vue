<template>
  <AppModal :show="show" @close="$emit('cancel')">
    <h3 class="text-lg font-medium text-foreground mb-2">{{ title }}</h3>
    <p class="text-sm text-muted-foreground mb-4">{{ message }}</p>

    <div v-if="requireText" class="mb-6">
      <label class="block text-sm font-medium text-foreground mb-1">
        Type <span class="font-mono font-bold text-foreground">{{ requireText }}</span> to confirm
      </label>
      <input
        v-model="typed"
        type="text"
        autocomplete="off"
        class="block w-full rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:border-destructive focus:ring-destructive sm:text-sm px-3 py-2"
        :placeholder="requireText"
      />
    </div>
    <div v-else class="mb-6" />

    <div class="flex justify-end gap-3">
      <AppButton variant="secondary" @click="cancel">Cancel</AppButton>
      <AppButton
        :variant="confirmVariant"
        :loading="loading"
        :disabled="!!requireText && typed !== requireText"
        @click="$emit('confirm')"
      >
        {{ confirmText }}
      </AppButton>
    </div>
  </AppModal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import AppModal from './AppModal.vue'
import AppButton from './AppButton.vue'

const props = withDefaults(
  defineProps<{
    show: boolean
    title?: string
    message?: string
    confirmText?: string
    confirmVariant?: 'primary' | 'danger'
    loading?: boolean
    requireText?: string
  }>(),
  {
    title: 'Are you sure?',
    message: 'This action cannot be undone.',
    confirmText: 'Confirm',
    confirmVariant: 'danger',
    loading: false,
    requireText: '',
  },
)

const emit = defineEmits<{
  confirm: []
  cancel: []
}>()

const typed = ref('')

// Reset typed input whenever the dialog is opened
watch(
  () => props.show,
  (val) => {
    if (val) typed.value = ''
  },
)

function cancel() {
  typed.value = ''
  emit('cancel')
}
</script>
