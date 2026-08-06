<template>
  <div class="max-w-4xl">
    <div class="flex items-center gap-3 mb-6">
      <RouterLink to="/contacts/duplicates" class="text-sm text-muted-foreground hover:text-foreground">
        ← Duplicates
      </RouterLink>
      <h1 class="text-2xl font-bold text-foreground">Merge contacts</h1>
    </div>

    <PageSpinner v-if="loading" />

    <!-- Gone: dismissed elsewhere, already merged, or never ours. Nothing to retry. -->
    <AppCard v-else-if="notFound">
      <div class="py-8 text-center space-y-3">
        <p class="text-foreground font-medium">This pair no longer exists</p>
        <p class="text-sm text-muted-foreground">It may have been merged or dismissed already.</p>
        <AppButton size="sm" variant="secondary" @click="goBack">Back to duplicates</AppButton>
      </div>
    </AppCard>

    <!-- Something went wrong on the way. Worth another attempt. -->
    <AppCard v-else-if="loadError">
      <div class="py-8 text-center space-y-3">
        <p class="text-foreground font-medium">Could not load this pair</p>
        <p class="text-sm text-muted-foreground">{{ loadError }}</p>
        <AppButton size="sm" variant="secondary" :loading="loading" @click="fetchDup"> Retry </AppButton>
      </div>
    </AppCard>

    <template v-else-if="dup && contactA && contactB">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <span
          class="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-orange-100 dark:bg-orange-500/20 text-orange-700 dark:text-orange-300"
        >
          {{ confidenceLabel(dup.score) }}
        </span>
        <span class="text-xs text-muted-foreground">
          {{ reasonLabels(dup.match_reasons).join(', ') }}
        </span>
      </div>

      <!-- Which record survives — an identity choice, kept separate from the values. -->
      <AppCard class="mb-6">
        <div class="mb-3">
          <h2 class="text-sm font-semibold text-foreground">Which record survives</h2>
          <p class="text-xs text-muted-foreground mt-1">
            The surviving record keeps its identifier, so devices already syncing it stay linked. This does
            not decide whose values are kept — choose those below.
          </p>
        </div>
        <div
          class="border border-border rounded-lg overflow-hidden grid grid-cols-1 sm:grid-cols-2 divide-y sm:divide-y-0 sm:divide-x divide-border"
        >
          <MergeContactColumn :contact="contactA" side="a" :selected="winner === 'a'" @select="setWinner" />
          <MergeContactColumn :contact="contactB" side="b" :selected="winner === 'b'" @select="setWinner" />
        </div>
      </AppCard>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div class="space-y-4">
          <h2 class="text-sm font-semibold text-foreground">Values to keep</h2>
          <MergeFieldGroup
            v-for="group in model.groups"
            :key="group.spec.property"
            :group="group"
            :selected="selection[group.spec.property] ?? []"
            @update="(ids) => (selection[group.spec.property] = ids)"
          />
          <p v-if="!model.groups.length" class="text-sm text-muted-foreground">
            These records hold no comparable values.
          </p>
        </div>

        <div class="space-y-4">
          <h2 class="text-sm font-semibold text-foreground">Result</h2>
          <MergePreviewCard :preview="preview" :discarded="discarded" />
        </div>
      </div>

      <p v-if="mergeError" class="mt-4 text-sm text-red-600 dark:text-red-400">{{ mergeError }}</p>

      <div class="mt-6 flex flex-wrap gap-3">
        <AppButton :loading="merging" @click="confirming = true">Merge contacts</AppButton>
        <AppButton variant="secondary" @click="goBack">Cancel</AppButton>
      </div>
    </template>

    <ConfirmDialog
      :show="confirming"
      title="Merge these contacts?"
      :message="confirmMessage"
      confirm-text="Merge"
      confirm-variant="danger"
      :loading="merging"
      @confirm="doMerge"
      @cancel="confirming = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { getDuplicate, mergeContacts } from '@/api/contacts'
import { getApiError } from '@/api/client'
import type { Contact, MergeSelection, PotentialDuplicate, ValueCandidate } from '@/types'
import {
  buildMergeModel,
  buildMergePayload,
  defaultSelection,
  discardedBySelection,
  previewFromSelection,
  type MergeModel,
} from '@/utils/merge'
import { confidenceLabel, reasonLabels } from '@/utils/duplicates'
import { useToast } from '@/composables/useToast'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import PageSpinner from '@/components/ui/PageSpinner.vue'
import MergeContactColumn from '@/components/contacts/MergeContactColumn.vue'
import MergeFieldGroup from '@/components/contacts/MergeFieldGroup.vue'
import MergePreviewCard from '@/components/contacts/MergePreviewCard.vue'

const route = useRoute()
const router = useRouter()
const toast = useToast()

const loading = ref(false)
const merging = ref(false)
const confirming = ref(false)
/** A pair that is gone is a different situation from a request that failed. */
const notFound = ref(false)
const loadError = ref('')
const mergeError = ref('')

const dup = ref<PotentialDuplicate | null>(null)
const candidates = ref<ValueCandidate[]>([])
const winner = ref<'a' | 'b'>('a')
const selection = reactive<MergeSelection>({})

const contactA = computed<Contact | null>(() => dup.value?.contact_a ?? null)
const contactB = computed<Contact | null>(() => dup.value?.contact_b ?? null)

const model = computed<MergeModel>(() => buildMergeModel(candidates.value))
const preview = computed(() => previewFromSelection(model.value, selection))
const discarded = computed(() => discardedBySelection(model.value, selection))

const confirmMessage = computed(() => {
  const kept = winner.value === 'a' ? contactA.value : contactB.value
  const dropped = winner.value === 'a' ? contactB.value : contactA.value
  const names = `"${nameOf(dropped)}" will be deleted and "${nameOf(kept)}" will remain`

  if (!discarded.value.length) return `${names}. No values are lost.`
  return `${names}. ${discarded.value.length} value(s) will be discarded.`
})

function nameOf(contact: Contact | null): string {
  if (!contact) return 'this contact'
  return [contact.first_name, contact.last_name].filter(Boolean).join(' ') || contact.email || 'unnamed'
}

function resetSelection() {
  for (const key of Object.keys(selection)) delete selection[key]
  Object.assign(selection, defaultSelection(model.value, winner.value))
}

function setWinner(side: 'a' | 'b') {
  winner.value = side
}

// Changing which record survives changes which single-valued defaults apply, so the
// selection is rebuilt. Multi-valued groups keep everything either way.
watch(winner, resetSelection)

async function fetchDup() {
  loading.value = true
  notFound.value = false
  loadError.value = ''

  try {
    // One request by id. The old view asked for a page of up to 200 pairs and searched it,
    // which meant a pair outside that page simply could not be opened.
    const { data } = await getDuplicate(route.params.dupId as string)
    dup.value = data.duplicate
    candidates.value = data.candidates ?? []
    winner.value = 'a'
    resetSelection()
  } catch (e: unknown) {
    const status = (e as { response?: { status?: number } })?.response?.status
    if (status === 404 || status === 403) {
      notFound.value = true
    } else {
      loadError.value = getApiError(e, 'Failed to load this pair')
    }
  } finally {
    loading.value = false
  }
}

async function doMerge() {
  if (!dup.value) return
  merging.value = true
  mergeError.value = ''

  try {
    await mergeContacts(
      buildMergePayload({
        dupId: dup.value.id,
        contactAId: dup.value.contact_a_id,
        contactBId: dup.value.contact_b_id,
        winner: winner.value,
        selection,
      }),
    )
    toast.success('Contacts merged')
    router.push('/contacts/duplicates')
  } catch (e: unknown) {
    // Stay on the page with the selection intact: navigating away would make the user
    // rebuild every choice to try again.
    mergeError.value = getApiError(e, 'Failed to merge contacts')
    confirming.value = false
  } finally {
    merging.value = false
  }
}

function goBack() {
  router.push('/contacts/duplicates')
}

onMounted(fetchDup)
</script>
