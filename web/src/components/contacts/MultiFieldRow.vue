<template>
  <div class="space-y-2">
    <label v-if="label" class="block text-sm font-medium text-foreground">{{ label }}</label>

    <div v-if="modelValue.length === 0" class="text-sm text-muted-foreground italic">No {{ addLabel || label?.toLowerCase() }}</div>

    <div
      v-for="(item, i) in modelValue"
      :key="i"
      class="flex items-center gap-2"
    >
      <!-- Type selector -->
      <select
        v-if="typeOptions && typeOptions.length"
        :value="item.type"
        class="w-28 flex-shrink-0 rounded-md border border-input bg-background text-foreground focus:border-ring focus:ring-ring sm:text-sm px-2 py-2"
        @change="updateType(i, ($event.target as HTMLSelectElement).value)"
      >
        <option value="">—</option>
        <option v-for="opt in typeOptions" :key="opt" :value="opt">
          {{ opt.charAt(0).toUpperCase() + opt.slice(1) }}
        </option>
      </select>

      <!-- Value input -->
      <input
        :type="inputType || 'text'"
        :value="item.value"
        :placeholder="placeholder"
        class="flex-1 block rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-ring sm:text-sm px-3 py-2"
        @input="updateValue(i, ($event.target as HTMLInputElement).value)"
      />

      <!-- Remove button -->
      <button
        type="button"
        class="flex-shrink-0 text-muted-foreground hover:text-destructive transition-colors p-1"
        :aria-label="`Remove ${label?.toLowerCase()}`"
        @click="remove(i)"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>

    <button
      type="button"
      class="text-sm text-accent hover:text-accent/80 font-medium"
      @click="add"
    >
      + Add {{ addLabel || label?.toLowerCase() }}
    </button>
  </div>
</template>

<script setup lang="ts">
import type { ContactFormField } from '@/types'

const props = defineProps<{
  modelValue: ContactFormField[]
  label?: string
  placeholder?: string
  inputType?: string
  typeOptions?: string[]
  addLabel?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: ContactFormField[]]
}>()

function updateValue(i: number, value: string) {
  const arr = [...props.modelValue]
  arr[i] = { ...arr[i], value }
  emit('update:modelValue', arr)
}

function updateType(i: number, type: string) {
  const arr = [...props.modelValue]
  arr[i] = { ...arr[i], type }
  emit('update:modelValue', arr)
}

function remove(i: number) {
  const arr = [...props.modelValue]
  arr.splice(i, 1)
  emit('update:modelValue', arr)
}

function add() {
  const defaultType = props.typeOptions?.[0] ?? ''
  emit('update:modelValue', [...props.modelValue, { value: '', type: defaultType }])
}
</script>
