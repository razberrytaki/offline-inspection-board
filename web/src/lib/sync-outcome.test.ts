import { describe, expect, it } from 'vitest';
import { parseLocalRecord } from './model';
import { classifyPut, retryDelay } from './sync-outcome';

const id = 'c3d402e2-ffad-4b92-9a14-facd07b19002';
const record = parseLocalRecord({
  id,
  authorId: 'worker-1',
  payload: { rootId: id, revision: 0, equipmentId: 'P-12', result: 'abnormal', memo: '누수', inspectedAt: 0 },
  sync: { kind: 'pending' }
});
const receivedAt = '2025-11-26T11:01:18.551654Z';

function response(status: number, body?: unknown) {
  return classifyPut({ kind: 'response', status, body }, record.id);
}

describe('점검 응답에 따른 기록 상태', () => {
  it.each([200, 201])('접수 시각을 저장하고 다음 기록을 보낸다 (HTTP %i)', (status) => {
    expect(response(status, { id, receivedAt })).toEqual({ kind: 'continue', sync: { kind: 'synced', receivedAt } });
  });

  it('서버가 UUID를 소문자로 반환해도 같은 기록의 접수로 처리한다', () => {
    const uppercaseId = id.toUpperCase();
    const uppercaseRecord = parseLocalRecord({ ...record, id: uppercaseId });
    expect(classifyPut({ kind: 'response', status: 200, body: { id, receivedAt } }, uppercaseRecord.id))
      .toEqual({ kind: 'continue', sync: { kind: 'synced', receivedAt } });
  });

  it('서버에서 받은 충돌 기록을 함께 저장한다', () => {
    const current = { ...record.payload, id, authorId: record.authorId, memo: '진동', receivedAt };
    expect(response(409, { reason: 'content_mismatch', current })).toEqual({
      kind: 'continue', sync: { kind: 'needs_attention', reason: 'content_mismatch', current }
    });
  });

  it.each(['content_mismatch', 'revision_taken', 'missing_previous'])('서버 기록이 없어도 충돌 사유를 저장한다 (%s)', (reason) => {
    expect(response(409, { reason })).toEqual({ kind: 'continue', sync: { kind: 'needs_attention', reason } });
  });

  it.each([[403, 'forbidden'], [422, 'invalid'], [409, 'invalid'], [404, 'invalid']])(
    '요청을 확인 필요로 표시하고 다음 기록을 보낸다 (HTTP %i)', (status, reason) => {
      expect(response(status)).toEqual({ kind: 'continue', sync: { kind: 'needs_attention', reason } });
    }
  );

  it('인증이 만료되면 기록을 그대로 두고 전송을 멈춘다', () => {
    expect(response(401)).toEqual({ kind: 'session_expired' });
  });

  it.each([429, 500, 503, 599])('일시적 서버 오류는 전송을 멈추고 재시도한다 (HTTP %i)', (status) => {
    expect(response(status)).toEqual({ kind: 'retry' });
  });

  it.each([undefined, '{', { id }, { id, receivedAt: '잘못된 시각' }, { id: '다른 ID', receivedAt }])(
    '접수 응답을 검증할 수 없으면 재시도한다 (%j)', (body) => {
      expect(response(200, body)).toEqual({ kind: 'retry' });
    }
  );

  it('네트워크 오류와 시간 초과는 전송을 멈추고 재시도한다', () => {
    expect(classifyPut({ kind: 'network_error' }, record.id)).toEqual({ kind: 'retry' });
    expect(classifyPut({ kind: 'timeout' }, record.id)).toEqual({ kind: 'retry' });
  });
});

describe('재시도 간격', () => {
  it('1초에서 두 배씩 늘리고 60초에서 멈춘다', () => {
    expect(Array.from({ length: 9 }, (_, attempt) => retryDelay(attempt, 0)))
      .toEqual([1000, 2000, 4000, 8000, 16000, 32000, 60000, 60000, 60000]);
  });

  it('무작위 지연을 더해도 60초를 넘지 않는다', () => {
    expect(retryDelay(1, 0.5)).toBe(2250);
    expect(retryDelay(6, 0.5)).toBe(60000);
  });
});
