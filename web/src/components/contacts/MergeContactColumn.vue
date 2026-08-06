<template>
  <label
    class="flex items-start gap-3 p-4 cursor-pointer transition-colors"
    :class="selected ? 'bg-blue-50 dark:bg-blue-500/10 ring-1 ring-inset ring-blue-400' : 'hover:bg-muted/50'"
  >
    <input
      type="radio"
      name="merge-winner"
      :value="side"
      :checked="selected"
      class="mt-1 shrink-0"
      :aria-label="`Keep ${label}`"
      @change="emit('select', side)"
    />
    <div class="min-w-0 space-y-1">
      <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {{ label }}
        <span v-if="selected" class="ml-1 text-blue-700 dark:text-blue-400">— survives</span>
      </p>
      <DuplicateSummary :contact="contact" />
      <p v-if="createdAt" class="text-xs text-muted-foreground">Added {{ createdAt }}</p>
    </div>
  </label>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Contact } from '@/types'
import DuplicateSummary from './DuplicateSummary.vue'
import { formatDate } from '@/utils/date'

/**
 * One side of the "which record survives" choice.
 *
 * This is about identity, not values: the winner keeps its UID, and every device that already
 * syncs this contact knows it by that UID. Which values end up on the card is a separate set
 * of choices further down the screen.
 */
const props = defineProps<{
  contact: Contact
  side: 'a' | 'b'
  selected: boolean
}>()

const emit = defineEmits<{ select: [side: 'a' | 'b'] }>()

const label = computed(() => (props.side === 'a' ? 'Record A' : 'Record B'))

const createdAt = computed(() => (props.contact.created_at ? formatDate(props.contact.created_at) : ''))
</script>
