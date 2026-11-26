import { describe, expect, it } from 'vitest';
import { createRecord, parseEquipmentList, parseLocalRecord, parseUser, searchEquipment } from './model';

const worker = parseUser({ userId: 'worker-1', name: '직원 1', role: 'worker' });
const equipment = parseEquipmentList([
  { id: 'P-12', name: '냉각수 펌프' },
  { id: 'F-08', name: '배기 팬' }
]);
const input = {
  id: 'c3d402e2-ffad-4b92-9a14-facd07b19002',
  authorId: worker.userId,
  equipmentId: equipment[0].id,
  result: 'abnormal',
  memo: '가동 중 누수 발견',
  inspectedAt: 1764130000000
} satisfies Parameters<typeof createRecord>[0];

describe('설비 검색', () => {
  it('번호는 대소문자와 앞뒤 공백을 무시하고, 설비명은 한글 부분 문자열로 검색한다', () => {
    expect(searchEquipment(equipment, ' p-12 ')).toEqual([equipment[0]]);
    expect(searchEquipment(equipment, '각수 펌'.normalize('NFD'))).toEqual([equipment[0]]);
    expect(searchEquipment(equipment, '')).toEqual(equipment);
    expect(searchEquipment(equipment, 'V-10')).toEqual([]);
  });
});

describe('점검 기록', () => {
  it('새 원본은 확정한 내용과 함께 전송 대기로 생성한다', () => {
    const record = createRecord(input);
    expect(record).toEqual({
      id: input.id,
      authorId: worker.userId,
      payload: {
        rootId: input.id, revision: 0, equipmentId: 'P-12', result: 'abnormal',
        memo: '가동 중 누수 발견', inspectedAt: 1764130000000
      },
      sync: { kind: 'pending' }
    });
    expect(parseLocalRecord(structuredClone(record))).toEqual(record);
  });

  it.each([
    { id: '잘못된 ID' },
    { memo: '   ' },
    { inspectedAt: 0.5 }
  ])('유효하지 않은 점검은 생성할 수 없다 (%j)', (patch) => {
    expect(() => createRecord({ ...input, ...patch })).toThrow();
  });

  it('기기에 저장된 잘못된 revision과 상태를 거부한다', () => {
    const record = createRecord(input);
    expect(() => parseLocalRecord({ ...record, payload: { ...record.payload, revision: 1 } })).toThrow();
    expect(() => parseLocalRecord({ ...record, sync: { kind: 'synced', receivedAt: 0 } })).toThrow();
    expect(() => parseLocalRecord({ ...record, sync: { kind: 'sending' } })).toThrow();
  });
});

describe('API 응답 해석', () => {
  it('계정과 설비 목록 응답을 확인한다', () => {
    expect(worker).toEqual({ userId: 'worker-1', name: '직원 1', role: 'worker' });
    expect(parseUser({ ...worker, role: 'supervisor' }).role).toBe('supervisor');
    expect(parseEquipmentList([])).toEqual([]);
    expect(equipment[0]).toEqual({ id: 'P-12', name: '냉각수 펌프' });
  });

  it.each([null, [], { userId: 1 }, { ...worker, role: 'admin' }, { ...worker, name: '' }].map(
    (value) => ({ value })
  ))(
    '잘못된 계정 응답을 거부한다 ($value)', ({ value }) => {
      expect(() => parseUser(value)).toThrow();
    }
  );

  it.each([null, {}, [{ id: '', name: '펌프' }], [{ id: 'P-12', name: 12 }], [...equipment, equipment[0]]].map(
    (value) => ({ value })
  ))('잘못된 설비 목록 응답을 거부한다 ($value)', ({ value }) => {
    expect(() => parseEquipmentList(value)).toThrow();
  });
});
