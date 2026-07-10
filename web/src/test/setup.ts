/**
 * Node exposes its own (disabled) `localStorage` global, which shadows the one jsdom
 * installs on `window`. Tests that exercise token storage need a working implementation,
 * so install a minimal in-memory Storage when the real one is unusable.
 */
function installLocalStorage() {
  const store = new Map<string, string>()
  const storage: Storage = {
    get length() {
      return store.size
    },
    clear: () => store.clear(),
    getItem: (key) => (store.has(key) ? store.get(key)! : null),
    key: (index) => Array.from(store.keys())[index] ?? null,
    removeItem: (key) => void store.delete(key),
    setItem: (key, value) => void store.set(key, String(value)),
  }

  Object.defineProperty(globalThis, 'localStorage', { value: storage, writable: true, configurable: true })
  Object.defineProperty(window, 'localStorage', { value: storage, writable: true, configurable: true })
}

if (typeof globalThis.localStorage?.getItem !== 'function') {
  installLocalStorage()
}
