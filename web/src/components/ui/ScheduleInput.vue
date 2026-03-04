<template>
  <div class="space-y-2">
    <label v-if="label" class="block text-sm font-medium text-gray-700">{{ label }}</label>

    <!-- Preset pills -->
    <div class="flex flex-wrap gap-2">
      <button
        v-for="preset in presets"
        :key="preset.value"
        type="button"
        @click="selectPreset(preset)"
        :disabled="disabled"
        class="px-3 py-1.5 text-xs font-medium rounded-full border transition-colors"
        :class="
          activePreset === preset.value
            ? 'bg-indigo-600 text-white border-indigo-600'
            : 'bg-white text-gray-600 border-gray-300 hover:border-indigo-400'
        "
      >
        {{ preset.label }}
      </button>
    </div>

    <!-- Custom cron input -->
    <div v-if="activePreset === 'custom'" class="mt-2">
      <label class="block text-xs font-medium text-gray-500 mb-1">
        Cron expression <span class="text-gray-400 font-normal">(UTC)</span>
      </label>
      <input
        :value="modelValue"
        @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        type="text"
        :disabled="disabled"
        placeholder="0 */6 * * *"
        class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 font-mono"
      />
    </div>

    <!-- Human-readable description -->
    <p v-if="modelValue" class="text-xs text-gray-400">
      {{ humanizeCron(modelValue) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { humanizeCron, type SchedulePreset } from '@/utils/cron'

const props = withDefaults(defineProps<{
  modelValue: string
  presets: SchedulePreset[]
  disabled?: boolean
  label?: string
}>(), {
  disabled: false,
  label: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const activePreset = ref('')

function syncActivePreset() {
  const match = props.presets.find(p => p.value === props.modelValue && p.value !== 'custom')
  activePreset.value = match ? match.value : (props.modelValue ? 'custom' : props.presets[0]?.value ?? '')
}

function selectPreset(preset: SchedulePreset) {
  activePreset.value = preset.value
  if (preset.value !== 'custom') {
    emit('update:modelValue', preset.value)
  }
}

onMounted(syncActivePreset)
watch(() => props.modelValue, syncActivePreset)
</script>
