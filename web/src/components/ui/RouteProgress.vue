<template>
  <div
    v-if="loading"
    class="fixed top-0 left-0 right-0 z-[100] h-0.5 bg-accent/20"
  >
    <div class="h-full bg-accent animate-progress" />
  </div>
</template>

<script setup lang="ts">
import { ref, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const loading = ref(false)
let timer: ReturnType<typeof setTimeout>

const removeBefore = router.beforeEach(() => {
  clearTimeout(timer)
  // Small delay to avoid flash on fast navigations
  timer = setTimeout(() => { loading.value = true }, 80)
})

const removeAfter = router.afterEach(() => {
  clearTimeout(timer)
  // Brief delay so the bar visually completes
  setTimeout(() => { loading.value = false }, 150)
})

onUnmounted(() => {
  removeBefore()
  removeAfter()
  clearTimeout(timer)
})
</script>

<style scoped>
@keyframes progress {
  0% { width: 0; }
  20% { width: 30%; }
  50% { width: 60%; }
  80% { width: 85%; }
  100% { width: 95%; }
}
.animate-progress {
  animation: progress 2s ease-out forwards;
}
</style>
