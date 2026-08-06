<template>
  <div class="space-y-4">
    <div class="border border-border rounded-lg overflow-hidden">
      <div class="px-4 py-2 bg-muted/50 border-b border-border">
        <span class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Result preview
        </span>
      </div>

      <div v-if="preview.length" class="divide-y divide-border">
        <div v-for="group in preview" :key="group.spec.property" class="px-4 py-2">
          <p class="text-xs text-muted-foreground">{{ group.spec.label }}</p>
          <p
            v-for="candidate in group.candidates"
            :key="candidate.id"
            class="text-sm text-foreground break-all"
          >
            {{ candidate.value }}
          </p>
        </div>
      </div>
      <p v-else class="px-4 py-3 text-sm text-muted-foreground">Nothing selected.</p>
    </div>

    <div
      class="border rounded-lg overflow-hidden"
      :class="discarded.length ? 'border-amber-300 dark:border-amber-500/40' : 'border-border'"
    >
      <div
        class="px-4 py-2 border-b"
        :class="
          discarded.length
            ? 'bg-amber-50 dark:bg-amber-500/10 border-amber-300 dark:border-amber-500/40'
            : 'bg-muted/50 border-border'
        "
      >
        <span class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {{ discarded.length ? `Will be discarded (${discarded.length})` : 'Nothing is lost' }}
        </span>
      </div>

      <!--
        The values are listed, not counted. "3 values will be discarded" tells someone that
        something is about to go without telling them whether they mind.
      -->
      <ul v-if="discarded.length" class="divide-y divide-border">
        <li v-for="candidate in discarded" :key="candidate.id" class="px-4 py-2">
          <p class="text-sm text-foreground break-all line-through">{{ candidate.value }}</p>
          <p class="text-xs text-muted-foreground">{{ propertyLabel(candidate.property) }}</p>
        </li>
      </ul>
      <p v-else class="px-4 py-3 text-sm text-muted-foreground">Every value from both records is kept.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ValueCandidate } from '@/types'
import { MERGE_FIELD_SPECS, type MergeFieldGroup } from '@/utils/merge'

/**
 * What the merge will produce, and what it will throw away.
 *
 * Both panels are derived from the current selection with no request to the server: the
 * question "what am I about to lose?" has to be answerable before committing, not after.
 */
defineProps<{
  preview: MergeFieldGroup[]
  discarded: ValueCandidate[]
}>()

function propertyLabel(property: string): string {
  return MERGE_FIELD_SPECS.find((s) => s.property === property)?.label ?? property
}
</script>
