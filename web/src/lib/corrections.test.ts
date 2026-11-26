import { expect, it } from 'vitest';
import { correctionBlock, createCorrection, groupRecords, nextRevision } from './corrections';
import { createRecord, parseEquipmentList, parseUser, type LocalRecord } from './model';

const [{ id: equipmentId }] = parseEquipmentList([{ id: 'P-12', name: '냉각수 펌프' }]);
const { userId: authorId } = parseUser({ userId: 'worker-1', name: '직원 1', role: 'worker' });
const original = createRecord({
  id: 'f1000000-0000-4000-8000-000000000000', authorId, equipmentId,
  result: 'abnormal', memo: '누수', inspectedAt: 1764130000000
});
const id = 'a1000000-0000-4000-8000-000000000000';
const edit = { result: 'normal', memo: '누수 아님' } satisfies Parameters<typeof createCorrection>[0]['edit'];
const [originalChain] = groupRecords([original]);
const correction = createCorrection({ chain: originalChain, id, edit });
const synced = { kind: 'synced', receivedAt: '2026-11-26T08:30:00Z' } satisfies LocalRecord['sync'];

it('원본과 정정을 한 항목으로 묶고 최신 내용과 전체 상태를 구한다', () => {
  const [chain, other] = groupRecords([
    correction, { ...original, sync: synced },
    createRecord({ ...original.payload, authorId, id: 'b1000000-0000-4000-8000-000000000000' })
  ]);
  expect(chain.history.map(({ id }) => id)).toEqual([original.id, correction.id]);
  expect(chain.latest.payload).toMatchObject(edit);
  expect(chain.state).toBe('pending');
  expect(other.history).toHaveLength(1);
  expect(nextRevision(chain)).toBe(2);
});

it('전송 대기 원본도 정정하며 새 ID와 차수를 쓰고 설비와 점검 시각을 유지한다', () => {
  expect(correctionBlock(originalChain)).toBe('');
  expect(correction).toEqual({
    id, authorId, sync: { kind: 'pending' },
    payload: { ...original.payload, ...edit, revision: 1 }
  });
  expect(original.payload.revision).toBe(0);
});

it('차수 하나라도 확인 필요이면 후속 정정을 막는다', () => {
  const [blocked] = groupRecords([correction, {
    ...original, sync: { kind: 'needs_attention', reason: 'content_mismatch' }
  }]);
  expect(blocked.state).toBe('needs_attention');
  expect(correctionBlock(blocked)).toContain('확인 필요');
  expect(() => createCorrection({ chain: blocked, id, edit })).toThrow('확인 필요');
});

it('삭제로 기기의 차수 이력이 끊긴 경우 정정을 막는다', () => {
  const [withoutOriginal] = groupRecords([correction]);
  expect(correctionBlock(withoutOriginal)).toContain('기기에 원본 또는 앞선 정정 기록이 없어');
});

it('변경 사항이 없거나 이상 결과에 메모가 없으면 정정 기록을 만들지 않는다', () => {
  expect(() => createCorrection({
    chain: originalChain, id, edit: { ...original.payload, memo: ' 누수 '.normalize('NFD') }
  })).toThrow('결과나 메모를 변경하세요.');
  expect(() => createCorrection({ chain: originalChain, id, edit: { result: 'abnormal', memo: '' } })).toThrow();
});
