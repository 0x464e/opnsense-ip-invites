import { createServer } from 'node:http'
import { test, expect } from '@playwright/test'

test('admin creates an invite using configured defaults; visitor authorizes; admin revokes', async ({
  page,
}, info) => {
  await page.setExtraHTTPHeaders({
    'X-Test-IP': info.project.name === 'mobile' ? '9.9.9.9' : '1.1.1.1',
  })
  await page.goto('/')
  await page.getByRole('link', { name: 'Administrator access' }).click()
  await expect(page.getByRole('heading', { name: 'Invitations', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'New invitation', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'New invitation' })
  await dialog.getByLabel('Access list').selectOption({ label: 'Service access' })
  await expect(dialog.getByLabel('Access per authorization')).toHaveValue('30')
  await expect(dialog.getByLabel('Access duration unit')).toHaveValue('60')
  await dialog.getByLabel('Label', { exact: true }).fill(`Browser test ${info.project.name}`)
  await dialog.getByRole('button', { name: 'Create invitation' }).click()
  const created = page.getByRole('dialog', { name: 'Your invitation is ready' })
  await expect(created).toBeVisible()
  const url = await created.getByLabel('Invitation link').inputValue()
  expect(url).toMatch(/\/i\/[\w-]{43}$/)
  await created.getByRole('button', { name: 'Done', exact: true }).click()
  await page.goto(url)
  await expect(page.getByRole('heading', { name: 'Service access' })).toBeVisible()
  await expect(page.getByText('30 minutes', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Authorize me' }).click()
  await expect(page.getByRole('heading', { name: 'You’re all set.' })).toBeVisible()
  await page.goto('/admin')
  await page
    .getByRole('button', { name: `Revoke Browser test ${info.project.name}`, exact: true })
    .click()
  const revoke = page.getByRole('dialog', { name: 'Revoke invitation', exact: true })
  await expect(revoke.getByLabel('Also revoke existing access')).toBeChecked()
  await revoke.getByRole('button', { name: 'Revoke', exact: true }).click()
  await expect(revoke).not.toBeVisible()
  await page.goto(url)
  await expect(page.getByRole('heading', { name: 'This invitation was revoked' })).toBeVisible()
})

test('IPv6 visits cannot authorize and show the configured IPv4 entry link', async ({ page }) => {
  const response = await page.request.get('http://127.0.0.1:18080/__fixture/invite')
  const { token } = await response.json()
  await page.setExtraHTTPHeaders({ 'X-Test-IP': '2001:db8::8' })
  await page.goto(`/i/${token}`)
  await expect(page.getByRole('button', { name: 'Authorize me' })).toBeDisabled()
  await expect(page.getByRole('alert')).toContainText('arrived over IPv6')
  await expect(page.getByRole('link', { name: 'Continue over IPv4' })).toHaveAttribute(
    'href',
    `http://127.0.0.1:5173/i/${token}`,
  )
})

test('errors never become a success state; admin grant view remains usable on small screens', async ({
  page,
}) => {
  const response = await page.request.get('http://127.0.0.1:18080/__fixture/invite')
  const { token } = await response.json()
  await page.route('**/authorize', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 'sync_pending',
        message: 'Authorization saved; firewall confirmation is pending.',
      }),
    }),
  )
  await page.goto(`/i/${token}`)
  await page.getByRole('button', { name: 'Authorize me' }).click()
  await expect(page.getByRole('alert')).toContainText('pending')
  await expect(page.getByRole('heading', { name: 'You’re all set.' })).not.toBeVisible()
  await page.goto('/admin')
  await page.getByRole('button', { name: 'Access grants', exact: true }).click()
  await expect(page.getByRole('cell', { name: '8.8.4.4', exact: true })).toBeVisible()
  const fits = await page.evaluate(
    () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
  )
  expect(fits).toBe(true)
})

test('admin creates a list, edits it, authorizes an invitation and deletes the list', async ({
  page,
}, info) => {
  const name = `Workspace ${info.project.name}`
  const alias = `WORKSPACE_${info.project.name.toUpperCase()}`
  await page.goto('/admin')
  await page.getByRole('button', { name: 'New list', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: 'New list', exact: true })
  await dialog.getByLabel('Name', { exact: true }).fill(name)
  await dialog.getByLabel('Description', { exact: true }).fill('Shared workspace access')
  await dialog.getByLabel('OPNsense alias', { exact: true }).fill(alias)
  await dialog.getByRole('button', { name: 'Create list', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible()
  await page.getByRole('button', { name: `Edit ${name}`, exact: true }).click()
  dialog = page.getByRole('dialog', { name: 'Edit list', exact: true })
  await expect(dialog.getByLabel('OPNsense alias')).toHaveAttribute('readonly', '')
  await dialog.getByLabel('Name', { exact: true }).fill(`${name} renamed`)
  await dialog.getByRole('button', { name: 'Save list', exact: true }).click()
  await expect(page.getByRole('heading', { name: `${name} renamed`, exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'New invitation', exact: true }).click()
  dialog = page.getByRole('dialog', { name: 'New invitation', exact: true })
  await dialog.getByLabel('Label', { exact: true }).fill(`Workspace invite ${info.project.name}`)
  await dialog.getByLabel('Access list', { exact: true }).selectOption({ label: `${name} renamed` })
  await dialog.getByRole('button', { name: 'Create invitation', exact: true }).click()
  const created = page.getByRole('dialog', { name: 'Your invitation is ready' })
  const url = await created.getByLabel('Invitation link').inputValue()
  await created.getByRole('button', { name: 'Done', exact: true }).click()
  await page.goto(url)
  await expect(page.getByRole('heading', { name: `${name} renamed`, exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Authorize me', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'You’re all set.' })).toBeVisible()
  await page.goto('/admin')
  await page.getByRole('button', { name: `Delete ${name} renamed`, exact: true }).click()
  dialog = page.getByRole('dialog', { name: 'Delete list', exact: true })
  await expect(dialog).toContainText('revoke all its invitations')
  await dialog.getByRole('button', { name: 'Delete list', exact: true }).click()
  await expect(
    page.getByRole('heading', { name: `${name} renamed`, exact: true }),
  ).not.toBeVisible()
  await page.goto(url)
  await expect(page.getByRole('heading', { name: 'This invitation was revoked' })).toBeVisible()
})

test('administrator access goes through a document request before any admin API calls', async ({
  page,
}) => {
  const adminApiRequests: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.startsWith('/api/admin/'))
      adminApiRequests.push(request.url())
  })
  const login = createServer((_request, response) => {
    response.writeHead(200, { 'Content-Type': 'text/html' })
    response.end('<!doctype html><title>Sign in</title><h1>Sign in</h1>')
  })
  await new Promise<void>((resolve) => login.listen(0, '127.0.0.1', resolve))
  try {
    const address = login.address()
    if (!address || typeof address === 'string')
      throw new Error('Missing test login server address')
    const loginURL = `http://127.0.0.1:${address.port}/login`
    // Model the proxy redirecting an unauthenticated document to an identity provider.
    await page.route('**/admin', async (route) => {
      expect(route.request().isNavigationRequest()).toBe(true)
      await route.fulfill({ status: 302, headers: { location: loginURL } })
    })
    await page.goto('/')
    await page.getByRole('link', { name: 'Administrator access' }).click()
    await expect(page).toHaveURL(loginURL)
    await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Invitations', exact: true })).not.toBeVisible()
    expect(adminApiRequests).toEqual([])
  } finally {
    await new Promise<void>((resolve, reject) =>
      login.close((error) => (error ? reject(error) : resolve())),
    )
  }
})
