<template>
  <div class="flex flex-wrap items-center gap-2">
    <!-- Category multi-select dropdown -->
    <div class="relative" ref="catDropdownRef">
      <button
        class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm border rounded-lg hover:bg-gray-50"
        :class="selectedCategories.length > 0 ? 'border-indigo-300 bg-indigo-50 text-indigo-700' : 'border-gray-300 text-gray-700'"
        @click="showCatDropdown = !showCatDropdown"
      >
        Tags
        <span
          v-if="selectedCategories.length > 0"
          class="inline-flex items-center justify-center w-5 h-5 text-xs font-medium rounded-full bg-indigo-600 text-white"
        >
          {{ selectedCategories.length }}
        </span>
        <svg class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
        </svg>
      </button>
      <div
        v-if="showCatDropdown && categories.length > 0"
        class="absolute z-30 mt-1 w-56 bg-white border border-gray-200 rounded-lg shadow-lg py-1 max-h-60 overflow-y-auto"
      >
        <label
          v-for="cat in categories"
          :key="cat"
          class="flex items-center px-3 py-1.5 hover:bg-gray-50 cursor-pointer text-sm"
        >
          <input
            type="checkbox"
            :checked="selectedCategories.includes(cat)"
            class="rounded border-gray-300 text-indigo-600 mr-2"
            @change="toggleCategory(cat)"
          />
          {{ cat }}
        </label>
      </div>
    </div>

    <!-- Org dropdown -->
    <div class="relative" ref="orgDropdownRef">
      <button
        class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm border rounded-lg hover:bg-gray-50"
        :class="selectedOrg ? 'border-indigo-300 bg-indigo-50 text-indigo-700' : 'border-gray-300 text-gray-700'"
        @click="showOrgDropdown = !showOrgDropdown"
      >
        {{ selectedOrg || 'Organization' }}
        <svg class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
        </svg>
      </button>
      <div
        v-if="showOrgDropdown && orgs.length > 0"
        class="absolute z-30 mt-1 w-56 bg-white border border-gray-200 rounded-lg shadow-lg py-1 max-h-60 overflow-y-auto"
      >
        <button
          v-if="selectedOrg"
          class="w-full text-left px-3 py-1.5 text-sm text-gray-400 hover:bg-gray-50"
          @click="$emit('update:org', ''); showOrgDropdown = false"
        >
          All organizations
        </button>
        <button
          v-for="org in orgs"
          :key="org"
          class="w-full text-left px-3 py-1.5 text-sm hover:bg-gray-50"
          :class="org === selectedOrg ? 'text-indigo-600 font-medium' : 'text-gray-700'"
          @click="$emit('update:org', org); showOrgDropdown = false"
        >
          {{ org }}
        </button>
      </div>
    </div>

    <!-- Has email pill -->
    <button
      class="px-3 py-1.5 text-sm border rounded-lg"
      :class="hasEmail ? 'border-indigo-300 bg-indigo-50 text-indigo-700' : 'border-gray-300 text-gray-700 hover:bg-gray-50'"
      @click="$emit('update:hasEmail', !hasEmail)"
    >
      Has email
    </button>

    <!-- Has phone pill -->
    <button
      class="px-3 py-1.5 text-sm border rounded-lg"
      :class="hasPhone ? 'border-indigo-300 bg-indigo-50 text-indigo-700' : 'border-gray-300 text-gray-700 hover:bg-gray-50'"
      @click="$emit('update:hasPhone', !hasPhone)"
    >
      Has phone
    </button>

    <!-- Clear filters -->
    <button
      v-if="activeCount > 0"
      class="px-3 py-1.5 text-sm text-indigo-600 hover:text-indigo-800"
      @click="$emit('clear')"
    >
      Clear filters
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'

const props = defineProps<{
  categories: string[]
  orgs: string[]
  selectedCategories: string[]
  selectedOrg: string
  hasEmail: boolean
  hasPhone: boolean
}>()

const emit = defineEmits<{
  'update:categories': [value: string[]]
  'update:org': [value: string]
  'update:hasEmail': [value: boolean]
  'update:hasPhone': [value: boolean]
  clear: []
}>()

const showCatDropdown = ref(false)
const showOrgDropdown = ref(false)
const catDropdownRef = ref<HTMLElement>()
const orgDropdownRef = ref<HTMLElement>()

const activeCount = computed(() => {
  let n = props.selectedCategories.length
  if (props.selectedOrg) n++
  if (props.hasEmail) n++
  if (props.hasPhone) n++
  return n
})

function toggleCategory(cat: string) {
  const idx = props.selectedCategories.indexOf(cat)
  const next = [...props.selectedCategories]
  if (idx >= 0) {
    next.splice(idx, 1)
  } else {
    next.push(cat)
  }
  emit('update:categories', next)
}

function handleClickOutside(e: MouseEvent) {
  if (catDropdownRef.value && !catDropdownRef.value.contains(e.target as Node)) {
    showCatDropdown.value = false
  }
  if (orgDropdownRef.value && !orgDropdownRef.value.contains(e.target as Node)) {
    showOrgDropdown.value = false
  }
}

onMounted(() => document.addEventListener('click', handleClickOutside))
onUnmounted(() => document.removeEventListener('click', handleClickOutside))
</script>
