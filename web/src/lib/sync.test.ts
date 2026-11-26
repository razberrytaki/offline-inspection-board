import { expect, it } from 'vitest';
import { parseLocalRecord } from './model';
import { pendingInSendOrder } from './sync';

function record(id: string, rootId = id, revision = 0, inspectedAt = 1764130000000) {
  return parseLocalRecord({
    id, authorId: 'worker-1', sync: { kind: 'pending' },
    payload: { rootId, revision, equipmentId: 'P-12', result: 'normal', memo: '', inspectedAt }
  });
}

it('같은 시각의 원본과 정정을 원본 ID와 차수 순서로 전송한다', () => {
  const original = record('f1000000-0000-4000-8000-000000000000');
  const first = record('b1000000-0000-4000-8000-000000000000', original.id, 1);
  const second = record('a1000000-0000-4000-8000-000000000000', original.id, 2);
  const other = record('c1000000-0000-4000-8000-000000000000');
  const older = record('f2000000-0000-4000-8000-000000000000', undefined, 0, original.payload.inspectedAt - 1);

  expect(pendingInSendOrder([second, first, other, original, older])).toEqual([
    older, other, original, first, second
  ]);
});
