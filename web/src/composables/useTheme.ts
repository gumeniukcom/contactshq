import { ref, computed, watchEffect } from 'vue'

type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'chq-theme'

const theme = ref<ThemeMode>((localStorage.getItem(STORAGE_KEY) as ThemeMode) || 'system')

const systemDark = ref(window.matchMedia('(prefers-color-scheme: dark)').matches)

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
  systemDark.value = e.matches
})

const resolved = computed<'light' | 'dark'>(() => {
  if (theme.value === 'system') return systemDark.value ? 'dark' : 'light'
  return theme.value
})

watchEffect(() => {
  const html = document.documentElement
  html.classList.remove('light', 'dark')
  html.classList.add(resolved.value)
  localStorage.setItem(STORAGE_KEY, theme.value)
})

export function useTheme() {
  function toggleTheme() {
    const modes: ThemeMode[] = ['light', 'dark', 'system']
    const idx = modes.indexOf(theme.value)
    theme.value = modes[(idx + 1) % modes.length]
  }

  function setTheme(mode: ThemeMode) {
    theme.value = mode
  }

  return { theme, resolved, toggleTheme, setTheme }
}
