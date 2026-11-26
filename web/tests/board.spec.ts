import { randomUUID } from 'node:crypto';
import { expect, type Page } from '@playwright/test';
import { test } from './fixtures';
import { login, ownRecords, pendingCount, readLocalRecords, savePump } from './field';
import { boardRow, boardRows, openBoard } from './supervisor';

function workerRow(page: Page, note: string) {
  return ownRecords(page).locator('ul.records > li').filter({ hasText: note });
}

async function recordPump(page: Page, note: string) {
  await savePump(page, note);
  const rows = await readLocalRecords(page);
  const record = rows.find(({ payload }) => payload.memo === note && payload.revision === 0);
  if (!record) throw new Error('기기에 저장한 원본을 찾을 수 없습니다.');
  return record;
}

test('정정하면 기존 행이 맨 위로 이동하고 같은 PUT의 재전송은 접수 시각을 바꾸지 않는다', async ({
  page, browser, baseURL
}) => {
  const supervisorContext = await browser.newContext({ baseURL });
  try {
    const supervisor = await openBoard(supervisorContext);
    await page.goto('/');
    await login(page, 'worker-1');

    const id = randomUUID();
    const firstMemo = '정정 전 ' + id;
    const correctedMemo = '정정 후 ' + id;
    const first = await recordPump(page, firstMemo);
    await expect(boardRow(supervisor, first.id)).toBeVisible();
    const second = await recordPump(page, '두 번째 점검 ' + id);
    const rows = boardRows(supervisor);
    await expect(rows.first()).toHaveAttribute('data-root-id', second.id);
    await expect(rows.nth(1)).toHaveAttribute('data-root-id', first.id);
    const count = await rows.count();

    await workerRow(page, firstMemo).getByRole('button', { name: '정정하기' }).click();
    const form = page.getByRole('form', { name: '점검 정정' });
    await form.getByRole('button', { name: '정정 저장' }).click();
    await expect(form.getByRole('alert')).toHaveText('결과나 메모를 변경하세요.');
    await form.getByLabel('정정 메모').fill(' ');
    await form.getByRole('button', { name: '정정 저장' }).click();
    await expect(form.getByLabel('정정 메모')).toHaveAttribute('aria-invalid', 'true');
    await expect(form.getByText('오류: 이상 결과에는 메모를 입력하세요.')).toBeVisible();

    await form.getByLabel('정정 메모').fill(correctedMemo);
    await form.getByRole('button', { name: '정정 저장' }).click();
    await expect(workerRow(page, correctedMemo).getByText('동기화 완료')).toBeVisible();

    await expect(rows.first()).toHaveAttribute('data-root-id', first.id);
    await expect(rows.first()).toHaveAttribute('data-revision', '1');
    await expect(rows.first()).toContainText(correctedMemo);
    await expect(rows.first()).toContainText('정정됨');
    await expect(rows).toHaveCount(count);
    const time = boardRow(supervisor, first.id).getByRole('cell').last().locator('time');
    const receivedAt = await time.getAttribute('datetime');
    const displayedTime = await time.innerText();

    await boardRow(supervisor, first.id).getByRole('link').click();
    await expect(supervisor).toHaveURL(new RegExp('/board/' + first.id + '$'));
    await expect(supervisor.getByRole('region', { name: '현재 기록' })).toContainText(correctedMemo);
    const history = supervisor.getByRole('region', { name: '점검 이력' }).getByRole('listitem');
    await expect(history).toHaveCount(2);
    await expect(history.nth(0)).toContainText('원본');
    await expect(history.nth(0)).toContainText(firstMemo);
    await expect(history.nth(1)).toContainText('정정 1');
    await expect(history.nth(1)).toContainText(correctedMemo);
    await supervisor.getByRole('link', { name: '보드로 돌아가기' }).click();
    await expect(supervisor.getByRole('status')).toHaveText('실시간 연결됨');

    const correction = (await readLocalRecords(page)).find(({ payload }) =>
      payload.rootId === first.id && payload.revision === 1
    );
    if (!correction) throw new Error('기기에 저장한 정정을 찾을 수 없습니다.');
    const replay = await page.request.put('/api/inspections/' + correction.id, { data: correction.payload });
    expect(replay.status()).toBe(200);
    expect(await replay.json()).toEqual({ id: correction.id, receivedAt });
    const third = await recordPump(page, '재전송 이후 점검 ' + id);
    await expect(rows.first()).toHaveAttribute('data-root-id', third.id);
    await expect(rows.nth(1)).toHaveAttribute('data-root-id', first.id);
    await expect(time).toHaveText(displayedTime);
    expect(await time.getAttribute('datetime')).toBe(receivedAt);
    await expect(rows).toHaveCount(count + 1);
  } finally {
    await supervisorContext.close();
  }
});

test('감독자 연결이 끊긴 사이 접수된 점검을 재연결 후 표시한다', async ({ page, browser, baseURL }) => {
  const supervisorContext = await browser.newContext({ baseURL });
  try {
    const supervisor = await openBoard(supervisorContext);
    await page.goto('/');
    await login(page, 'worker-1');

    await supervisorContext.setOffline(true);
    await expect(supervisor.getByRole('status')).toHaveText('다시 연결 중');
    const note = '감독자 연결 끊김 중 접수 ' + randomUUID();
    const record = await recordPump(page, note);
    await expect(workerRow(page, note).getByText('동기화 완료')).toBeVisible();
    await expect(boardRow(supervisor, record.id)).toHaveCount(0);

    await supervisorContext.setOffline(false);
    await expect(supervisor.getByRole('status')).toHaveText('실시간 연결됨');
    await expect(boardRow(supervisor, record.id)).toContainText(note);
  } finally {
    await supervisorContext.close();
  }
});

test('오프라인에서 연달아 저장한 원본과 정정을 차수 순서대로 전송한다', async ({
  page, context, browser, baseURL
}) => {
  const supervisorContext = await browser.newContext({ baseURL });
  try {
    const supervisor = await openBoard(supervisorContext);
    await page.goto('/');
    await login(page, 'worker-1');
    await context.setOffline(true);
    await expect(page.getByText('오프라인', { exact: true })).toBeVisible();

    await supervisor.getByRole('radio', { name: '이상', exact: true }).check();
    const note = '오프라인 원본 ' + randomUUID();
    const correctedMemo = '오프라인 정정 ' + randomUUID();
    const original = await recordPump(page, note);
    await expect(workerRow(page, note).getByText('전송 대기')).toBeVisible();
    await workerRow(page, note).getByRole('button', { name: '정정하기' }).click();
    const form = page.getByRole('form', { name: '점검 정정' });
    await form.getByRole('radio', { name: '정상' }).check();
    await form.getByLabel('정정 메모').fill(correctedMemo);
    await form.getByRole('button', { name: '정정 저장' }).click();
    await expect(workerRow(page, correctedMemo).getByRole('button', { name: '정정하기' })).toBeVisible();
    await expect(ownRecords(page).locator('ul.records > li')).toHaveCount(1);
    await expect(pendingCount(page)).toHaveText('2건');
    await workerRow(page, correctedMemo).getByText('점검 이력 (2건)').click();
    const revisions = workerRow(page, correctedMemo).getByRole('list', { name: '점검 차수' }).getByRole('listitem');
    await expect(revisions.nth(0)).toContainText('원본');
    await expect(revisions.nth(0)).toContainText('전송 대기');
    await expect(revisions.nth(1)).toContainText('정정 1');
    await expect(revisions.nth(1)).toContainText('전송 대기');

    await context.setOffline(false);
    await expect(revisions.nth(0)).toContainText('동기화 완료');
    await expect(revisions.nth(1)).toContainText('동기화 완료');
    await expect(pendingCount(page)).toHaveText('0건');
    await supervisor.getByRole('radio', { name: '전체', exact: true }).check();
    await expect(boardRow(supervisor, original.id)).toHaveAttribute('data-revision', '1');
    await expect(boardRow(supervisor, original.id)).toContainText(correctedMemo);
    await supervisor.getByRole('radio', { name: '이상', exact: true }).check();
    await expect(boardRow(supervisor, original.id)).toHaveCount(0);
  } finally {
    await supervisorContext.close();
  }
});

test('로그아웃 상태와 직원은 보드를 볼 수 없고 감독자는 로그인 후 보드로 이동한다', async ({
  page, browser, baseURL
}) => {
  const supervisorContext = await browser.newContext({ baseURL });
  try {
    await page.goto('/board');
    await expect(page.getByRole('heading', { name: '감독자 로그인 필요' })).toBeVisible();
    await expect(page.getByRole('table')).toHaveCount(0);
    await page.getByRole('link', { name: '로그인 화면으로' }).click();
    await login(page, 'worker-1');
    await expect(page).toHaveURL(/\/$/);
    await page.goto('/board');
    await expect(page.getByRole('heading', { name: '감독자 로그인 필요' })).toBeVisible();
    await expect(page.getByRole('table')).toHaveCount(0);

    const supervisor = await openBoard(supervisorContext);
    await supervisor.goto('/');
    await expect(supervisor).toHaveURL(/\/board$/);
    await expect(supervisor.getByRole('status')).toHaveText('실시간 연결됨');

    await supervisorContext.setOffline(true);
    await expect(supervisor.getByRole('status')).toHaveText('다시 연결 중');
    const logout = await supervisor.request.post('/api/session/logout');
    expect(logout.status()).toBe(204);
    await supervisorContext.setOffline(false);
    await expect(supervisor.getByRole('heading', { name: '감독자 로그인 필요' })).toBeVisible();
    await expect(supervisor.getByRole('table')).toHaveCount(0);
  } finally {
    await supervisorContext.close();
  }
});
