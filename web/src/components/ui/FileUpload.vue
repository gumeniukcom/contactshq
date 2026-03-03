<template>
  <div
    class="border-2 border-dashed rounded-lg p-8 text-center transition-colors"
    :class="dragOver ? 'border-indigo-500 bg-indigo-50' : 'border-gray-300 hover:border-gray-400'"
    @dragover.prevent="dragOver = true"
    @dragleave="dragOver = false"
    @drop.prevent="onDrop"
  >
    <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12" />
    </svg>
    <p class="mt-2 text-sm text-gray-600">
      Drag and drop your file here, or
      <label class="text-indigo-600 hover:text-indigo-500 cursor-pointer font-medium">
        browse
        <input type="file" class="hidden" :accept="accept" @change="onFileChange" />
      </label>
    </p>
    <p v-if="fileName" class="mt-2 text-sm font-medium text-gray-900">{{ fileName }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

defineProps<{ accept?: string }>()

const emit = defineEmits<{
  file: [file: File]
}>()

const dragOver = ref(false)
const fileName = ref('')

function onDrop(e: DragEvent) {
  dragOver.value = false
  const file = e.dataTransfer?.files[0]
  if (file) {
    fileName.value = file.name
    emit('file', file)
  }
}

function onFileChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (file) {
    fileName.value = file.name
    emit('file', file)
  }
}
</script>
