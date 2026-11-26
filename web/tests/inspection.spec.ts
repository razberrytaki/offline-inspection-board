import { expect, test, type Page, type Request } from '@playwright/test';
import { parseLocalRecord } from '../src/lib/model';

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

async function submitPump(page: Page, note = memo) {
  await selectAbnormalPump(page);
  await page.getByLabel('메모').fill(note);
  await page.getByRole('button', { name: '기기에 저장', exact: true }).click();
}

async function savePump(page: Page, note = memo) {
  await submitPump(page, note);
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
  const ownRecords = page.getByRole('region', { name: '내 기록' });
  await expect(ownRecords.getByText(memo)).toBeVisible();
  await expect(ownRecords.getByText('동기화 완료')).toBeVisible();
  await expect(pendingCount(page)).toHaveText('0건');
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

function ownRecords(page: Page) {
  return page.getByRole('region', { name: '내 기록' });
}

function inspectionPut(request: Request) {
  return request.method() === 'PUT' && new URL(request.url()).pathname.startsWith('/api/inspections/');
}

async function readLocalRecords(page: Page) {
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

test('저장 직후 응답이 끊겨도 재시도한 점검 한 건만 보드에 접수된다', async ({ page, context, playwright, baseURL }) => {
  const note = '냉각수 펌프 응답 유실 검증';
  await page.goto('/');
  await login(page, 'worker-1');
  await context.setOffline(true);
  await expect(page.getByText('오프라인', { exact: true })).toBeVisible();
  await savePump(page, note);
  const row = ownRecords(page).getByRole('listitem');
  await expect(row.getByText('전송 대기')).toBeVisible();

  const [record] = await readLocalRecords(page);
  const fault = await page.request.post('/api/dev/faults', { data: { dropAfterCommit: 1 } });
  expect(fault.status()).toBe(204);

  const dropped = page.waitForEvent('requestfailed', {
    predicate: (request) => inspectionPut(request) && request.failure()?.errorText === 'net::ERR_EMPTY_RESPONSE'
  });
  const replayed = page.waitForResponse((response) => inspectionPut(response.request()) && response.status() === 200);
  await context.setOffline(false);
  const [failedRequest, receipt] = await Promise.all([dropped, replayed]);
  expect(failedRequest.postData()).toBe(JSON.stringify(record.payload));
  expect(receipt.request().postData()).toBe(failedRequest.postData());
  await expect(row.getByText('동기화 완료')).toBeVisible();
  await expect(pendingCount(page)).toHaveText('0건');

  const supervisor = await playwright.request.newContext({ baseURL });
  try {
    const session = await supervisor.post('/api/session', {
      data: { userId: 'supervisor', password: 'demo-supervisor' }
    });
    expect(session.status()).toBe(204);
    const board = await supervisor.get('/api/board');
    await expect(board).toBeOK();
    const rows: { rootId: string }[] = await board.json();
    expect(rows.filter((row) => row.rootId === record.id)).toEqual([
      expect.objectContaining({ equipmentId: 'P-12', result: 'abnormal', memo: note })
    ]);
  } finally {
    await supervisor.dispose();
  }
});

test('만료된 세션에서 전송 대기를 유지하고 다시 로그인하면 전송한다', async ({ page }) => {
  await page.goto('/');
  await login(page, 'worker-1');

  await page.route('**/api/inspections/*', async (route) => {
    const logout = await page.request.post('/api/session/logout');
    expect(logout.status()).toBe(204);
    await route.continue();
  }, { times: 1 });
  const expired = page.waitForResponse((response) => inspectionPut(response.request()) && response.status() === 401);
  await submitPump(page);
  await expired;

  await expect(page.getByRole('status').getByText('다시 로그인 필요')).toBeVisible();
  await expect(ownRecords(page).getByText('전송 대기')).toBeVisible();
  await expect(pendingCount(page)).toHaveText('1건');
  expect((await readLocalRecords(page))[0].sync).toEqual({ kind: 'pending' });

  await login(page, 'worker-1');
  await expect(ownRecords(page).getByText('동기화 완료')).toBeVisible();
  await expect(pendingCount(page)).toHaveText('0건');
});

test('충돌한 두 기록을 비교하고 새 원본을 제출한 뒤 기기 기록을 삭제한다', async ({ page, context }) => {
  await page.goto('/');
  await login(page, 'worker-1');
  await context.setOffline(true);
  await savePump(page);
  const row = ownRecords(page).getByRole('listitem');
  await expect(row.getByText('전송 대기')).toBeVisible();

  const [record] = await readLocalRecords(page);
  const serverMemo = '서버에 먼저 접수된 기록';
  const seeded = await page.request.put('/api/inspections/' + record.id, {
    data: { ...record.payload, result: 'normal', memo: serverMemo }
  });
  expect(seeded.status()).toBe(201);
  await context.setOffline(false);

  const attention = ownRecords(page).getByRole('button', { name: '확인 필요', exact: true });
  await attention.click();
  const details = page.getByRole('region', { name: '확인 필요 상세' });
  await expect(details.getByText('같은 점검 ID에 다른 내용이 이미 접수되었습니다.')).toBeVisible();
  const local = details.getByRole('heading', { name: '기기 기록' }).locator('..');
  await expect(local.getByText(memo)).toBeVisible();
  await expect(local.getByText('이상')).toBeVisible();
  const current = details.getByRole('heading', { name: '서버 기록' }).locator('..');
  await expect(current.getByText(serverMemo)).toBeVisible();
  await expect(current.getByText('정상')).toBeVisible();

  await details.getByRole('button', { name: '새 점검으로 다시 제출' }).click();
  await expect(ownRecords(page).getByText('동기화 완료')).toBeVisible();
  await expect(ownRecords(page).getByRole('listitem')).toHaveCount(2);
  const resubmitted = (await readLocalRecords(page)).find(({ id }) => id !== record.id);
  expect(resubmitted).toEqual(expect.objectContaining({
    payload: { ...record.payload, rootId: resubmitted?.id },
    sync: expect.objectContaining({ kind: 'synced' })
  }));

  await attention.click();
  const remove = details.getByRole('button', { name: '이 기기에서 삭제' });
  await remove.click();
  const dialog = page.getByRole('dialog', { name: '이 기기에서 점검 기록을 삭제할까요?' });
  await expect(dialog.getByRole('button', { name: '취소' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await expect(remove).toBeFocused();
  await expect(ownRecords(page).getByRole('listitem')).toHaveCount(2);

  await remove.click();
  await dialog.getByRole('button', { name: '삭제', exact: true }).click();
  await expect(ownRecords(page).getByRole('listitem')).toHaveCount(1);
  await expect(ownRecords(page).getByText('동기화 완료')).toBeVisible();
  expect((await readLocalRecords(page)).map(({ id }) => id)).toEqual([resubmitted?.id]);
});
