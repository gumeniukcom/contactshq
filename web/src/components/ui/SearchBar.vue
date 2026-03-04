<template>
  <div class="relative">
    <input
      :value="modelValue"
      type="text"
      :placeholder="placeholder"
      class="block w-full rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-ring sm:text-sm pl-10 pr-8 py-2"
      @input="onInput"
    />
    <svg
      class="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
    >
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
    </svg>
    <button
      v-if="modelValue"
      class="absolute right-2 top-2.5 text-muted-foreground hover:text-foreground"
      @click="$emit('update:modelValue', '')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
let timer: ReturnType<typeof setTimeout>

defineProps<{
  modelValue: string
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

function onInput(e: Event) {
  const value = (e.target as HTMLInputElement).value
  clearTimeout(timer)
  timer = setTimeout(() => emit('update:modelValue', value), 300)
}
</script>
