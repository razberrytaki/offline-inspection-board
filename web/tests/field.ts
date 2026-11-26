import { expect, type Page, type Request } from '@playwright/test';
import { parseLocalRecord } from '../src/lib/model';

export const memo = '가동 중 누수 발견';

export async function signIn(page: Page, userId: 'worker-1' | 'worker-2' | 'supervisor') {
  await page.getByLabel('아이디').fill(userId);
  await page.getByLabel('비밀번호').fill('demo-' + userId);
  await page.getByRole('button', { name: '로그인', exact: true }).click();
}

export async function login(page: Page, worker: 'worker-1' | 'worker-2') {
  await signIn(page, worker);
  await expect(page.getByRole('heading', { name: '점검 기록하기' })).toBeVisible();
  await expect(page.getByText('3대', { exact: true })).toBeVisible();
}

export function pendingCount(page: Page) {
  return page.getByRole('banner').getByText('전송 대기', { exact: true }).locator('..').getByRole('definition');
}

export async function selectAbnormalPump(page: Page) {
  await page.getByRole('searchbox', { name: '설비 검색' }).fill('P-12');
  await page.getByRole('radio', { name: 'P-12 냉각수 펌프' }).check();
  await page.getByRole('radio', { name: '이상', exact: true }).check();
}

export async function submitPump(page: Page, note = memo) {
  await selectAbnormalPump(page);
  await page.getByLabel('메모').fill(note);
  await page.getByRole('button', { name: '기기에 저장', exact: true }).click();
}

export async function savePump(page: Page, note = memo) {
  await submitPump(page, note);
  await expect(page.locator('.notice.saved')).toHaveText('기기에 저장됨');
  await expect(page.getByLabel('메모')).toHaveValue('');
}

export function ownRecords(page: Page) {
  return page.getByRole('region', { name: '내 기록' });
}

export function inspectionPut(request: Request) {
  return request.method() === 'PUT' && new URL(request.url()).pathname.startsWith('/api/inspections/');
}

export async function readLocalRecords(page: Page) {
  const stored = await page.evaluate(async () => {
    const db = await new Promise<IDBDatabase>((resolve, reject) => {
      const request = indexedDB.open('oib');
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
    try {
      return await new Promise<unknown[]>((resolve, reject) => {
        const request = db.transaction('records').objectStore('records').getAll();
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      });
    } finally {
      db.close();
    }
  });
  return stored.map(parseLocalRecord);
}
