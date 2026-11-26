import { expect, test, type Page } from '@playwright/test';

const memo = '가동 중 누수 발견';

async function login(page: Page, worker: 'worker-1' | 'worker-2') {
  await page.getByLabel('아이디').fill(worker);
  await page.getByLabel('비밀번호').fill('demo-' + worker);
  await page.getByRole('button', { name: '로그인', exact: true }).click();
  await expect(page.getByRole('heading', { name: '점검 기록하기' })).toBeVisible();
  await expect(page.getByText('3대', { exact: true })).toBeVisible();
}

function pendingCount(page: Page) {
  return page.getByRole('banner').getByText('전송 대기', { exact: true }).locator('..').getByRole('definition');
}

async function selectAbnormalPump(page: Page) {
  await page.getByRole('searchbox', { name: '설비 검색' }).fill('P-12');
  await page.getByRole('radio', { name: 'P-12 냉각수 펌프' }).check();
  await page.getByRole('radio', { name: '이상', exact: true }).check();
}

async function savePump(page: Page) {
  await selectAbnormalPump(page);
  await page.getByLabel('메모').fill(memo);
  await page.getByRole('button', { name: '기기에 저장', exact: true }).click();
  await expect(page.locator('.notice.saved')).toHaveText('기기에 저장됨');
  await expect(page.getByLabel('메모')).toHaveValue('');
}

test('오프라인에서 저장한 점검이 다시 열어도 전송 대기로 남는다', async ({ page, context }) => {
  await page.goto('/');
  await login(page, 'worker-1');
  await page.waitForFunction(() => navigator.serviceWorker.controller !== null);

  await context.setOffline(true);
  await expect(page.getByText('오프라인', { exact: true })).toBeVisible();
  await savePump(page);

  const response = await page.reload();
  expect(response?.fromServiceWorker()).toBe(true);
  await expect(page.getByText('오프라인', { exact: true })).toBeVisible();
  const record = page.getByRole('region', { name: '내 기록' }).getByRole('listitem');
  await expect(record).toContainText('P-12 냉각수 펌프');
  await expect(record).toContainText('이상');
  await expect(record).toContainText(memo);
  await expect(record.getByText('전송 대기', { exact: true })).toBeVisible();
  await expect(pendingCount(page)).toHaveText('1건');

  await page.close();
  const reopened = await context.newPage();
  const navigation = await reopened.goto('/');
  expect(navigation?.fromServiceWorker()).toBe(true);
  const reopenedRecord = reopened.getByRole('region', { name: '내 기록' }).getByRole('listitem');
  await expect(reopenedRecord).toContainText(memo);
  await expect(reopenedRecord.getByText('전송 대기', { exact: true })).toBeVisible();
  await expect(pendingCount(reopened)).toHaveText('1건');
});

test('같은 기기에서 다른 직원은 이전 직원의 기록을 볼 수 없다', async ({ page }) => {
  await page.goto('/');
  await login(page, 'worker-1');
  await savePump(page);
  await page.getByRole('button', { name: '로그아웃' }).click();

  await login(page, 'worker-2');
  await expect(page.getByRole('banner').getByText('직원 2')).toBeVisible();
  await expect(page.getByRole('region', { name: '내 기록' }).getByText(memo)).toHaveCount(0);
  await expect(pendingCount(page)).toHaveText('0건');

  await page.getByRole('button', { name: '로그아웃' }).click();
  await login(page, 'worker-1');
  await expect(page.getByRole('region', { name: '내 기록' }).getByText(memo)).toBeVisible();
  await expect(pendingCount(page)).toHaveText('1건');
});

test('이상 결과의 빈 메모에 오류를 표시하고 기록을 저장하지 않는다', async ({ page }) => {
  await page.goto('/');
  await login(page, 'worker-1');
  await selectAbnormalPump(page);

  const field = page.getByLabel('메모');
  const error = page.locator('#memo-error');
  await expect(error).toBeHidden();
  await expect(field).toHaveAttribute('aria-invalid', 'false');
  await expect(field).not.toHaveAttribute('aria-describedby');
  const submit = page.getByRole('button', { name: '기기에 저장', exact: true });
  await expect(submit).toBeEnabled();
  await submit.click();

  await expect(error).toBeVisible();
  await expect(error).toHaveText('오류: 이상 결과에는 메모를 입력하세요.');
  await expect(field).toBeFocused();
  await expect(field).toHaveAttribute('aria-invalid', 'true');
  await expect(field).toHaveAttribute('aria-describedby', 'memo-error');
  await expect(page.getByRole('region', { name: '내 기록' }).getByRole('listitem')).toHaveCount(0);
  await expect(pendingCount(page)).toHaveText('0건');
  await field.blur();
  await expect(page.locator('[aria-live="polite"]').first()).toHaveText('오류: 이상 결과에는 메모를 입력하세요.');
  await field.fill(memo);
  await expect(field).toHaveAttribute('aria-invalid', 'false');
  await expect(field).not.toHaveAttribute('aria-describedby');

  await page.reload();
  await expect(pendingCount(page)).toHaveText('0건');
  await expect(page.getByRole('region', { name: '내 기록' }).getByRole('listitem')).toHaveCount(0);
});
