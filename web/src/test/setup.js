import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

const storage = new Map()

Object.defineProperty(window, 'localStorage', {
  value: {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, String(value)),
    removeItem: (key) => storage.delete(key),
    clear: () => storage.clear(),
  },
  writable: true,
})

afterEach(() => {
  cleanup()
  storage.clear()
})
