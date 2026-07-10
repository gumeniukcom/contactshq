<template>
  <div class="flex items-baseline gap-3 group">
    <dt class="w-28 shrink-0 text-xs uppercase tracking-wide text-muted-foreground">{{ label }}</dt>
    <dd class="flex-1 min-w-0 flex items-center gap-2">
      <a v-if="href" :href="href" class="text-accent hover:underline break-all">{{ value }}</a>
      <span v-else class="text-foreground break-all whitespace-pre-wrap">{{ value }}</span>
      <button
        class="opacity-0 group-hover:opacity-100 focus:opacity-100 text-muted-foreground hover:text-foreground shrink-0"
        :aria-label="`Copy ${label}`"
        @click="copy"
      >
        <span v-if="copied" class="text-xs text-accent">Copied</span>
        <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
          />
        </svg>
      </button>
    </dd>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ label: string; value: string; href?: string }>()

const copied = ref(false)

async function copy() {
  try {
    await navigator.clipboard.writeText(props.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    // Clipboard access can be denied; the value is selectable either way.
    copied.value = false
  }
}
</script>
