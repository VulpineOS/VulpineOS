async (page) => {
  const env = typeof process !== 'undefined' && process?.env ? process.env : {}
  const panelURL = env.VULPINE_PANEL_SMOKE_URL || 'http://127.0.0.1:8443'
  const accessKey = env.VULPINE_PANEL_SMOKE_ACCESS_KEY || ''
  const screenshotPath =
    env.VULPINE_PANEL_SMOKE_SCREENSHOT || '/tmp/vulpineos-panel-smoke.png'

  await page.setViewportSize({ width: 1440, height: 960 })
  await page.goto(panelURL, { waitUntil: 'domcontentloaded' })

  const accessKeyField = page.getByLabel(/access key/i)
  if ((await accessKeyField.count()) > 0) {
    if (!accessKey) {
      throw new Error('panel smoke reached login without VULPINE_PANEL_SMOKE_ACCESS_KEY')
    }
    await accessKeyField.fill(accessKey)
    await page.getByRole('button', { name: /connect/i }).click()
  }

  await page.waitForLoadState('networkidle')
  await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
  await page.getByText(/Connected|Connecting|Reconnecting/i).first().waitFor()
  await page.getByRole('link', { name: 'Agents', exact: true }).click()
  await page.getByRole('heading', { name: 'Agents', exact: true }).waitFor()
  await page.getByRole('button', { name: 'Refresh', exact: true }).waitFor()
  await page.getByRole('link', { name: 'Settings', exact: true }).click()
  await page.getByRole('heading', { name: 'Settings', exact: true }).waitFor()

  await page.locator('main').screenshot({ path: screenshotPath })

  return {
    ok: true,
    page: 'panel',
    url: page.url(),
    screenshotPath,
  }
}
