import { expect, type Page } from '@playwright/test';
import { test } from './fixtures';
import { inspectionPut, login, ownRecords, pendingCount, readLocalRecords, savePump } from './field';
import { boardRow, openBoard } from './supervisor';
import { normalURL } from './ports';

async function openPanel(page: Page) {
  const summary = page.getByText('장애 패널', { exact: true });
  await expect(summary).toBeVisible();
  await summary.click();
  const panel = page.locator('details').filter({ has: summary });
  await expect(panel.getByText(/서버 점검 행 수/)).toBeVisible();
  return panel;
}

test('장애 패널로 서버 연결을 끊고 복구하면 전송 대기 점검이 동기화된다', async ({ page, browser, baseURL }) => {
  const supervisorContext = await browser.newContext({ baseURL });
  try {
    const board = await openBoard(supervisorContext);
    await page.goto('/');
    await login(page, 'worker-1');
    let panel = await openPanel(page);
    await panel.getByRole('button', { name: '서버 연결 끊기' }).click();
    await expect(page.getByRole('banner').getByText('오프라인', { exact: true })).toBeVisible();
    await expect(board.getByRole('status')).toHaveText('다시 연결 중');

    await savePump(page);
    await expect(ownRecords(page).getByText('전송 대기')).toBeVisible();
    await expect(pendingCount(page)).toHaveText('1건');
    const [record] = await readLocalRecords(page);
    await expect(panel.getByRole('listitem').filter({ hasText: record.id }).first()).toContainText('offline');
    await expect(boardRow(board, record.id)).toHaveCount(0);

    await page.reload();
    await expect(ownRecords(page).getByText('전송 대기')).toBeVisible();
    panel = await openPanel(page);
    await panel.getByRole('button', { name: '서버 연결 복구' }).click();
    await expect(ownRecords(page).getByText('동기화 완료')).toBeVisible();
    await expect(pendingCount(page)).toHaveText('0건');
    await expect(board.getByRole('status')).toHaveText('실시간 연결됨');
    await expect(boardRow(board, record.id)).toHaveCount(1);
  } finally {
    await supervisorContext.close();
  }
});

test('장애 패널에 응답 유실과 재전송 200이 기록되고 서버 행이 한 건 늘어난다', async ({ page }) => {
  await page.goto('/');
  await login(page, 'worker-1');
  const panel = await openPanel(page);
  const count = panel.getByText(/서버 점검 행 수/);
  const before = Number(await count.locator('strong').innerText());
  await panel.getByRole('button', { name: '저장 직후 응답 끊기' }).click();
  await expect(panel.getByText('응답 끊기 예약 1건')).toBeVisible();
  await savePump(page);
  await expect(ownRecords(page).getByText('동기화 완료')).toBeVisible();
  const [record] = await readLocalRecords(page);
  const attempts = panel.getByRole('listitem').filter({ hasText: record.id }).locator('strong');
  await expect(attempts).toHaveText(['dropped', '200']);
  await expect(count).toHaveText('서버 점검 행 수 ' + (before + 1) + '건');
  await expect(panel.getByText('응답 끊기 예약 1건')).toHaveCount(0);
});

test('장애 패널에서 점검을 다섯 번 동시 전송해도 서버 행 수가 유지된다', async ({ page }) => {
  await page.goto('/');
  await login(page, 'worker-1');
  const panel = await openPanel(page);
  const created = page.waitForResponse((response) => inspectionPut(response.request()) && response.status() === 201);
  const count = panel.getByText(/서버 점검 행 수/);
  const before = Number(await count.locator('strong').innerText());
  await savePump(page);
  const original = (await created).request();
  await expect(ownRecords(page).getByText('동기화 완료')).toBeVisible();
  await expect(count).toHaveText('서버 점검 행 수 ' + (before + 1) + '건');

  const replays: (string | null)[] = [];
  page.on('request', (request) => {
    if (inspectionPut(request) && request.url() === original.url()) replays.push(request.postData());
  });
  await panel.getByRole('button', { name: '같은 점검 5번 동시 전송' }).click();
  await expect(panel.getByRole('status')).toHaveText('동시 전송 결과 200 · 200 · 200 · 200 · 200');
  expect(replays).toEqual(Array(5).fill(original.postData()));

  const [record] = await readLocalRecords(page);
  const attempts = panel.getByRole('listitem').filter({ hasText: record.id }).locator('strong');
  await expect(attempts).toHaveText(['201', '200', '200', '200', '200', '200']);
  await expect(count).toHaveText('서버 점검 행 수 ' + (before + 1) + '건');
});

test('장애 기능을 켜지 않은 서버에서는 직원 화면에 장애 패널이 없다', async ({ page }) => {
  await page.goto(normalURL);
  const denied = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/dev/faults');
  await login(page, 'worker-1');
  expect((await denied).status()).toBe(404);
  await expect(page.getByText('장애 패널', { exact: true })).toHaveCount(0);
});
