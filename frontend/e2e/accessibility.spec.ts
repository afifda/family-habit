import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

const publicRoutes = ['/login', '/register', '/parent/unlock', '/missing-page'];

for (const route of publicRoutes) {
  test(`A11Y-AUTO public route ${route} has no serious axe violations`, async ({
    page,
  }) => {
    await page.goto(route);
    await page.waitForLoadState('networkidle');
    await expect(page.locator('main, section').first()).toBeVisible();
    const result = await new AxeBuilder({ page }).analyze();
    const blockers = result.violations.filter(({ impact }) =>
      ['serious', 'critical'].includes(impact ?? ''),
    );
    expect(blockers, JSON.stringify(blockers, null, 2)).toEqual([]);
  });
}

test('A11Y-KB-01 public authentication has logical visible focus', async ({
  page,
}) => {
  await page.goto('/login');
  await page.keyboard.press('Tab');
  await expect(page.getByRole('link', { name: /Habit Home/i })).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.getByLabel('Email address')).toBeFocused();
  await expect(page.getByLabel('Email address')).toHaveCSS(
    'outline-style',
    'solid',
  );
});

test('A11Y-REFLOW-01 public setup remains operable at 320 CSS pixels', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto('/register');
  await expect(
    page.getByRole('heading', { name: 'Create your family space' }),
  ).toBeVisible();
  await expect(page.getByRole('button', { name: 'Continue' })).toBeVisible();
  const overflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth >
      document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);
});

test('A11Y-TARGET-01 primary controls provide practical 48px targets', async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto('/login');
  const target = await page
    .getByRole('button', { name: 'Sign in' })
    .boundingBox();
  expect(target?.width).toBeGreaterThanOrEqual(48);
  expect(target?.height).toBeGreaterThanOrEqual(48);
});

test('A11Y-MOTION-01 reduced motion removes non-essential transitions', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/login');
  const duration = await page
    .getByRole('button', { name: 'Sign in' })
    .evaluate((element) => getComputedStyle(element).transitionDuration);
  expect(Number.parseFloat(duration)).toBeLessThanOrEqual(0.00001);
});
