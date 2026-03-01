import { test, expect } from '@playwright/test'

/**
 * Smoke test: verifies that clicking "Start Session" eventually attaches
 * a <video> element with a live stream src, or the WebSocket fallback
 * renders an <img> with a data-URI src.
 *
 * Prerequisites:
 *   - `make dev` is running (all services up).
 *   - BASE_URL env var set to the frontend origin (default: http://localhost:5173).
 */
test.describe('Browser Isolation – smoke', () => {
  test('Start Session attaches a video or fallback image element', async ({ page }) => {
    await page.goto('/')

    // Fill in a target URL.
    await page.fill('input[type="url"]', 'https://example.com')

    // Click the Start Session button.
    await page.click('button:has-text("Start Session")')

    // The UI should transition away from idle – look for status indicators.
    await expect(page.locator('text=Starting…').or(page.locator('text=starting'))).toBeVisible({
      timeout: 10_000,
    })

    // Wait up to 45 s for the session to become ready (runner + Chromium startup).
    await expect(page.locator('text=ready')).toBeVisible({ timeout: 45_000 })

    // The WebRTC stream path: a <video> element should appear and have a srcObject.
    // We check that the video element exists and is not hidden.
    const video = page.locator('video')
    const img = page.locator('img[src^="data:image"]')

    // Wait for either the WebRTC video or the WS fallback image to appear.
    await expect(video.or(img)).toBeVisible({ timeout: 20_000 })

    // Verify the video element has srcObject set (WebRTC active) OR img has data src (WS fallback).
    const hasStream = await page.evaluate(() => {
      const v = document.querySelector('video')
      if (v && v.srcObject) return true
      const img = document.querySelector('img[src^="data:image"]')
      return img !== null
    })
    expect(hasStream).toBe(true)

    // End the session cleanly.
    await page.click('button:has-text("End Session")')
    await expect(page.locator('button:has-text("Start Session")')).toBeVisible({ timeout: 5_000 })
  })

  test('Session status endpoint returns session data', async ({ request }) => {
    // Create a session via the API directly.
    const createRes = await request.post('http://localhost:8090/api/sessions', {
      data: { url: 'https://example.com' },
    })
    expect(createRes.status()).toBe(201)

    const body = await createRes.json()
    expect(body).toHaveProperty('sessionId')
    expect(body.status).toBe('starting')

    const { sessionId } = body

    // Poll until ready or timeout (30 s).
    let ready = false
    for (let i = 0; i < 20; i++) {
      await new Promise((r) => setTimeout(r, 1500))
      const statusRes = await request.get(`http://localhost:8090/api/sessions/${sessionId}`)
      const status = await statusRes.json()
      if (status.status === 'ready') {
        ready = true
        break
      }
    }
    expect(ready).toBe(true)

    // Clean up.
    const deleteRes = await request.delete(`http://localhost:8090/api/sessions/${sessionId}`)
    expect(deleteRes.status()).toBe(204)
  })
})
