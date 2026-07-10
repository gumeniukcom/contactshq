import { ref } from 'vue'

export interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'info'
}

// Module-level state: every caller shares one queue, and AppToast renders it once.
const toasts = ref<Toast[]>([])
let nextId = 0

const DISMISS_AFTER_MS = 4000

function push(message: string, type: Toast['type']) {
  const id = nextId++
  toasts.value.push({ id, message, type })
  setTimeout(() => dismiss(id), DISMISS_AFTER_MS)
}

function dismiss(id: number) {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

export function useToast() {
  return {
    toasts,
    dismiss,
    success: (message: string) => push(message, 'success'),
    error: (message: string) => push(message, 'error'),
    info: (message: string) => push(message, 'info'),
  }
}
