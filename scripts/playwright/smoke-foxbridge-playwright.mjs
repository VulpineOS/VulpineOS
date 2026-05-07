import { createRequire } from 'node:module'
import Module from 'node:module'

const endpoint = process.env.VULPINE_FOXBRIDGE_WS || 'ws://127.0.0.1:9222/devtools/browser/foxbridge'
const require = createRequire(import.meta.url)

let chromium
try {
  Module._initPaths()
  ;({ chromium } = require('playwright'))
} catch (err) {
  console.error('Playwright is required. Install it with: npm install --no-save playwright')
  process.exit(1)
}

try {
  const browser = await chromium.connectOverCDP(endpoint)
  const context = browser.contexts()[0] || await browser.newContext()
  console.log(`Playwright connected through foxbridge: ${endpoint}`)
  console.log(`Contexts: ${browser.contexts().length}; pages in first context: ${context.pages().length}`)
  await browser.close()
} catch (err) {
  console.error(err instanceof Error ? err.stack : err)
  process.exit(1)
}
