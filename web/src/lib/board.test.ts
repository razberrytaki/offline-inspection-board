import { expect, it } from 'vitest';
import { applyRow, parseBoardList, parseRow, visibleRows } from './board';

const input = {
  rootId: 'a1000000-0000-4000-8000-000000000000', revision: 0,
  equipmentId: 'P-12', equipmentName: '냉각수 펌프', result: 'abnormal', memo: '누수',
  authorId: 'worker-1', authorName: '직원 1', inspectedAt: 1764130000000,
  receivedAt: '2026-11-26T11:00:00.123001Z'
};
const original = parseRow(input);
const correction = parseRow({ ...input, revision: 1, result: 'normal', memo: '누수 아님' });

it('같은 원본의 높은 차수만 반영하고 기존 Map은 그대로 둔다', () => {
  const before = applyRow(new Map(), original);
  const after = applyRow(before, correction);
  expect(before.get(original.rootId)).toEqual(original);
  expect([...after.values()]).toEqual([correction]);
  expect(applyRow(after, { ...correction, memo: '다른 내용' })).toBe(after);
  expect(applyRow(after, original)).toBe(after);
});

it('밀리초 미만까지 포함해 최근 접수순으로 정렬하고 시각이 같으면 원본 ID 순으로 정렬한다', () => {
  const newer = parseRow({
    ...input, rootId: 'c1000000-0000-4000-8000-000000000000', receivedAt: '2026-11-26T11:00:00.123002Z'
  });
  const sameTime = parseRow({ ...input, rootId: 'b1000000-0000-4000-8000-000000000000' });
  expect(visibleRows(parseBoardList([sameTime, original, newer]), 'all')).toEqual([newer, original, sameTime]);
});

it('실시간 정정으로 바뀐 현재 결과를 기준으로 이상 필터를 적용한다', () => {
  const before = parseBoardList([original]);
  expect(visibleRows(before, 'abnormal')).toEqual([original]);
  expect(visibleRows(applyRow(before, correction), 'abnormal')).toEqual([]);
});

it('같은 원본이 목록에 여러 번 있으면 높은 차수만 사용한다', () => {
  expect([...parseBoardList([correction, original]).values()]).toEqual([correction]);
});

it.each([
  { rootId: '잘못된 ID' }, { revision: -1 }, { revision: 1.5 }, { equipmentId: '' },
  { equipmentName: null }, { result: 'unknown' }, { memo: '' }, { memo: 42 },
  { authorId: '' }, { authorName: 1 }, { inspectedAt: '1764130000000' }, { receivedAt: '어제' }
])('잘못된 보드 행을 거부한다 (%j)', (patch) => {
  expect(parseRow(input)).toEqual(input);
  expect(() => parseRow({ ...input, ...patch })).toThrow();
  expect(() => parseBoardList([input, { ...input, ...patch }])).toThrow();
});
