export type InspectionId = string & { readonly __brand: 'InspectionId' };
export type UserId = string & { readonly __brand: 'UserId' };
export type EquipmentId = string & { readonly __brand: 'EquipmentId' };

export type User = { userId: UserId; name: string; role: 'worker' | 'supervisor' };
export type Equipment = { id: EquipmentId; name: string };
export type Result = 'normal' | 'abnormal';

type Payload = {
  rootId: InspectionId;
  revision: number;
  equipmentId: EquipmentId;
  result: Result;
  memo: string;
  inspectedAt: number;
};

type ServerRecord = Payload & { id: InspectionId; authorId: UserId; receivedAt: string };

type SyncState =
  | { kind: 'pending' }
  | { kind: 'synced'; receivedAt: string }
  | { kind: 'needs_attention'; reason: string; current?: ServerRecord }
  | { kind: 'needs_login' };

export type LocalRecord = {
  id: InspectionId;
  authorId: UserId;
  payload: Payload;
  sync: SyncState;
};

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isText(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0 && !value.includes('\u0000');
}

export function isUserId(value: unknown): value is UserId {
  return isText(value);
}

export function isEquipmentId(value: unknown): value is EquipmentId {
  return isText(value);
}

function isInspectionId(value: unknown): value is InspectionId {
  return typeof value === 'string' && value.length === 36 &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
}

export function isResult(value: unknown): value is Result {
  return value === 'normal' || value === 'abnormal';
}

export function isTimestamp(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) &&
    value >= -210_866_803_200_000 && value <= 8_640_000_000_000_000;
}

function isReceivedAt(value: unknown): value is string {
  return typeof value === 'string' &&
    /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(value) &&
    Number.isFinite(Date.parse(value));
}

export function parseUser(value: unknown): User {
  if (!isObject(value) || !isUserId(value.userId) || !isText(value.name) ||
      (value.role !== 'worker' && value.role !== 'supervisor')) {
    throw new Error('사용자 정보 형식이 올바르지 않습니다.');
  }
  return { userId: value.userId, name: value.name, role: value.role };
}

export function parseEquipmentList(value: unknown): Equipment[] {
  if (!Array.isArray(value)) throw new Error('설비 목록 형식이 올바르지 않습니다.');

  const ids = new Set<EquipmentId>();
  return Array.from(value, (item: unknown) => {
    if (!isObject(item) || !isEquipmentId(item.id) || !isText(item.name) || ids.has(item.id)) {
      throw new Error('설비 목록 형식이 올바르지 않습니다.');
    }
    ids.add(item.id);
    return { id: item.id, name: item.name };
  });
}

function parsePayload(value: unknown, id: InspectionId): Payload {
  if (!isObject(value)) throw new Error('점검 내용 형식이 올바르지 않습니다.');
  const { rootId, revision, equipmentId, result, memo, inspectedAt } = value;
  if (!isInspectionId(rootId) || typeof revision !== 'number' ||
      !Number.isInteger(revision) || revision < 0 || revision > 2_147_483_647 ||
      (revision === 0) !== (rootId.toLowerCase() === id.toLowerCase()) ||
      !isEquipmentId(equipmentId) || !isResult(result) || typeof memo !== 'string' ||
      memo.includes('\u0000') || (result === 'abnormal' && !memo.trim()) || !isTimestamp(inspectedAt)) {
    throw new Error('점검 내용 형식이 올바르지 않습니다.');
  }
  return { rootId, revision, equipmentId, result, memo, inspectedAt };
}

function parseServerRecord(value: unknown): ServerRecord {
  if (!isObject(value) || !isInspectionId(value.id) ||
      !isUserId(value.authorId) || !isReceivedAt(value.receivedAt)) {
    throw new Error('서버 점검 기록 형식이 올바르지 않습니다.');
  }
  return {
    ...parsePayload(value, value.id),
    id: value.id,
    authorId: value.authorId,
    receivedAt: value.receivedAt
  };
}

function parseSync(value: unknown): SyncState {
  if (isObject(value)) {
    switch (value.kind) {
      case 'pending':
      case 'needs_login':
        return { kind: value.kind };
      case 'synced':
        if (isReceivedAt(value.receivedAt)) {
          return { kind: value.kind, receivedAt: value.receivedAt };
        }
        break;
      case 'needs_attention':
        if (isText(value.reason)) {
          return {
            kind: value.kind,
            reason: value.reason,
            current: value.current === undefined ? undefined : parseServerRecord(value.current)
          };
        }
        break;
    }
  }
  throw new Error('점검 기록 상태 형식이 올바르지 않습니다.');
}

export function parseLocalRecord(value: unknown): LocalRecord {
  if (!isObject(value) || !isInspectionId(value.id) || !isUserId(value.authorId)) {
    throw new Error('점검 기록 형식이 올바르지 않습니다.');
  }
  return {
    id: value.id,
    authorId: value.authorId,
    payload: parsePayload(value.payload, value.id),
    sync: parseSync(value.sync)
  };
}

export function createRecord(input: {
  id: string;
  authorId: UserId;
  equipmentId: EquipmentId;
  result: Result;
  memo: string;
  inspectedAt: number;
}): LocalRecord {
  const { id, authorId, equipmentId, result, memo, inspectedAt } = input;
  return parseLocalRecord({
    id,
    authorId,
    payload: { rootId: id, revision: 0, equipmentId, result, memo, inspectedAt },
    sync: { kind: 'pending' }
  });
}

export function searchEquipment(items: Equipment[], query: string): Equipment[] {
  const term = query.trim().normalize('NFC').toLowerCase();
  return items.filter(({ id, name }) =>
    id.normalize('NFC').toLowerCase().includes(term) ||
    name.normalize('NFC').toLowerCase().includes(term)
  );
}
