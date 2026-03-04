<template>
  <div
    v-if="resolvedSrc"
    :class="sizeClass"
    class="rounded-full overflow-hidden flex-shrink-0"
  >
    <img :src="resolvedSrc" :alt="initials" class="w-full h-full object-cover" />
  </div>
  <div
    v-else
    :class="[sizeClass, textClass]"
    :style="{ backgroundColor: bgColor }"
    class="rounded-full flex items-center justify-center flex-shrink-0 font-medium text-white"
  >
    {{ initials }}
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  firstName?: string
  lastName?: string
  photoUri?: string
  size?: 'sm' | 'md' | 'lg'
}>(), {
  firstName: '',
  lastName: '',
  size: 'md',
})

const resolvedSrc = computed(() => {
  const uri = props.photoUri
  if (!uri) return ''
  // Already a proper URL or data URI
  if (uri.startsWith('http://') || uri.startsWith('https://') || uri.startsWith('data:')) return uri
  // Raw base64 — detect JPEG/PNG magic and add data URI prefix
  if (uri.startsWith('/9j/')) return `data:image/jpeg;base64,${uri}`
  if (uri.startsWith('iVBOR')) return `data:image/png;base64,${uri}`
  // Unknown format — skip to avoid broken requests
  return ''
})

const palette = [
  '#6366f1', '#8b5cf6', '#ec4899', '#f43f5e', '#ef4444',
  '#f97316', '#eab308', '#22c55e', '#14b8a6', '#0ea5e9',
]

const initials = computed(() => {
  const f = props.firstName?.trim()?.[0] ?? ''
  const l = props.lastName?.trim()?.[0] ?? ''
  return (f + l).toUpperCase() || '?'
})

const bgColor = computed(() => {
  const name = (props.firstName + props.lastName).toLowerCase()
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return palette[Math.abs(hash) % palette.length]
})

const sizeClass = computed(() => ({
  sm: 'h-8 w-8',
  md: 'h-10 w-10',
  lg: 'h-12 w-12',
}[props.size]))

const textClass = computed(() => ({
  sm: 'text-xs',
  md: 'text-sm',
  lg: 'text-base',
}[props.size]))
</script>
