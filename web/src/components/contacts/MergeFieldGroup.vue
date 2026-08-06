<template>
  <div class="border border-border rounded-lg overflow-hidden">
    <div class="px-4 py-2 bg-muted/50 border-b border-border flex items-center gap-2">
      <span class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {{ group.spec.label }}
      </span>
      <!--
        Difference is marked with words, not only a colour: a yellow background was the sole
        signal before, which says nothing to anyone who cannot distinguish it.
      -->
      <span
        v-if="group.differs"
        class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-amber-100 dark:bg-amber-500/20 text-amber-800 dark:text-amber-300"
      >
        differs
      </span>
      <span v-else class="text-xs text-muted-foreground">identical</span>
    </div>

    <!-- SINGLETON: mutually exclusive choices. -->
    <div
      v-if="group.spec.arity === 'SINGLETON'"
      class="flex flex-col sm:flex-row divide-y sm:divide-y-0 sm:divide-x divide-border"
    >
      <label
        v-for="candidate in group.candidates"
        :key="candidate.id"
        class="flex-1 flex items-start gap-3 p-4 cursor-pointer transition-colors"
        :class="
          isSelected(candidate.id)
            ? 'bg-blue-50 dark:bg-blue-500/10 ring-1 ring-inset ring-blue-400'
            : 'hover:bg-muted/50'
        "
      >
        <input
          type="radio"
          :name="`field-${group.spec.property}`"
          :value="candidate.id"
          :checked="isSelected(candidate.id)"
          class="mt-0.5 shrink-0"
          @change="emit('update', [candidate.id])"
        />
        <div class="min-w-0">
          <p class="text-xs font-medium text-muted-foreground mb-1">{{ sideLabel(candidate) }}</p>
          <p class="text-sm text-foreground break-all">{{ candidate.value }}</p>
        </div>
      </label>
    </div>

    <!-- MULTI: every value can be kept; all are on by default. -->
    <div v-else class="divide-y divide-border">
      <label
        v-for="candidate in group.candidates"
        :key="candidate.id"
        class="flex items-start gap-3 p-3 cursor-pointer transition-colors"
        :class="isSelected(candidate.id) ? 'bg-blue-50/50 dark:bg-blue-500/5' : 'hover:bg-muted/50'"
      >
        <input
          type="checkbox"
          :checked="isSelected(candidate.id)"
          class="mt-0.5 shrink-0"
          @change="toggle(candidate.id)"
        />
        <div class="min-w-0 flex-1">
          <p class="text-sm text-foreground break-all">{{ candidate.value }}</p>
          <p class="text-xs text-muted-foreground">
            <span>{{ sideLabel(candidate) }}</span>
            <span v-if="paramLabel(candidate)"> · {{ paramLabel(candidate) }}</span>
          </p>
        </div>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ValueCandidate } from '@/types'
import type { MergeFieldGroup } from '@/utils/merge'

/**
 * One property's worth of choices.
 *
 * Single-valued properties get radios because keeping two display names is not a thing a card
 * can do; multi-valued ones get checkboxes, all ticked, because a merge that quietly dropped
 * one of two phone numbers would be the worst possible default.
 */
const props = defineProps<{
  group: MergeFieldGroup
  selected: string[]
}>()

const emit = defineEmits<{ update: [ids: string[]] }>()

function isSelected(id: string): boolean {
  return props.selected.includes(id)
}

function toggle(id: string) {
  const next = isSelected(id) ? props.selected.filter((x) => x !== id) : [...props.selected, id]
  emit('update', next)
}

function sideLabel(candidate: ValueCandidate): string {
  if (candidate.side === 'both') return 'In both'
  return candidate.side === 'winner' ? 'Record A' : 'Record B'
}

function paramLabel(candidate: ValueCandidate): string {
  const type = candidate.params?.TYPE ?? candidate.params?.type
  return type ? type.replace(/,/g, ', ') : ''
}
</script>
