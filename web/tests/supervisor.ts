import { expect, type BrowserContext, type Page } from '@playwright/test';
import { signIn } from './field';

export async function openBoard(context: BrowserContext) {
  const page = await context.newPage();
  await page.goto('/');
  await signIn(page, 'supervisor');
  await expect(page).toHaveURL(/\/board$/);
  await expect(page.getByRole('status')).toHaveText('실시간 연결됨');
  return page;
}

export function boardRows(page: Page) {
  return page.getByRole('table', { name: '서버 접수 점검' }).locator('tbody').getByRole('row');
}

export function boardRow(page: Page, rootId: string) {
  return page.getByRole('table', { name: '서버 접수 점검' }).locator(`tr[data-root-id="${rootId}"]`);
}
