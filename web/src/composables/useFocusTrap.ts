import { onBeforeUnmount, watch, type Ref } from 'vue'

const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function focusable(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => el.offsetParent !== null,
  )
}

/**
 * Keep keyboard focus inside an open dialog and restore it afterwards.
 *
 * Without this, Tab walks straight out of the dialog into the page behind the overlay
 * and Escape does nothing — a keyboard user cannot tell where they are, and cannot leave.
 */
export function useFocusTrap(container: Ref<HTMLElement | null>, open: Ref<boolean>, onEscape: () => void) {
  let previouslyFocused: HTMLElement | null = null

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onEscape()
      return
    }
    if (e.key !== 'Tab' || !container.value) return

    const items = focusable(container.value)
    if (items.length === 0) {
      e.preventDefault()
      return
    }

    const first = items[0]
    const last = items[items.length - 1]
    const active = document.activeElement as HTMLElement | null

    if (e.shiftKey && (active === first || !container.value.contains(active))) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && active === last) {
      e.preventDefault()
      first.focus()
    }
  }

  function activate() {
    previouslyFocused = document.activeElement as HTMLElement | null
    document.addEventListener('keydown', onKeydown, true)
    // Wait for the dialog's children to render before reaching for one.
    requestAnimationFrame(() => {
      if (!container.value) return
      const items = focusable(container.value)
      ;(items[0] ?? container.value).focus()
    })
  }

  function deactivate() {
    document.removeEventListener('keydown', onKeydown, true)
    previouslyFocused?.focus()
    previouslyFocused = null
  }

  watch(open, (isOpen) => (isOpen ? activate() : deactivate()), { immediate: true })
  onBeforeUnmount(deactivate)
}
