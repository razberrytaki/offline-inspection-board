import { openDB, type DBSchema, type IDBPDatabase } from 'idb';
import {
  type Equipment, type InspectionId, type LocalRecord, type User, type UserId,
  isTimestamp, parseEquipmentList, parseLocalRecord, parseUser
} from './model';
import { createCorrection, groupRecords, type InspectionEdit } from './corrections';
import type { FinishedSync } from './sync-outcome';

interface InspectionDB extends DBSchema {
  equipment: { key: string; value: unknown };
  meta: { key: 'user' | 'equipmentSyncedAt' | 'logoutPending'; value: unknown };
  records: { key: string; value: unknown; indexes: { byAuthor: string } };
}

let database: Promise<IDBPDatabase<InspectionDB>> | undefined;

function open() {
  database ??= openDB<InspectionDB>('oib', 1, {
    upgrade(db) {
      db.createObjectStore('equipment', { keyPath: 'id' });
      db.createObjectStore('meta');
      db.createObjectStore('records', { keyPath: 'id' }).createIndex('byAuthor', 'authorId');
    }
  });
  return database;
}

export async function readSession() {
  const db = await open();
  const tx = db.transaction('meta');
  const [user, logoutPending] = await Promise.all([
    tx.store.get('user'), tx.store.get('logoutPending'), tx.done
  ]);
  if (logoutPending !== undefined && logoutPending !== true) {
    throw new Error('기기에 저장된 로그아웃 상태가 올바르지 않습니다.');
  }
  return {
    user: user === undefined ? null : parseUser(user),
    logoutPending: logoutPending === true
  };
}

export async function cacheUser(user: User | null) {
  const db = await open();
  if (user === null) await db.delete('meta', 'user');
  else await db.put('meta', user, 'user');
}

export async function beginLogout() {
  const db = await open();
  const tx = db.transaction('meta', 'readwrite');
  await Promise.all([
    tx.store.delete('user'),
    tx.store.put(true, 'logoutPending'),
    tx.done
  ]);
}

export async function completeLogout() {
  const db = await open();
  await db.delete('meta', 'logoutPending');
}

export async function readEquipment() {
  const db = await open();
  const tx = db.transaction(['equipment', 'meta']);
  const [equipment, syncedAt] = await Promise.all([
    tx.objectStore('equipment').getAll(),
    tx.objectStore('meta').get('equipmentSyncedAt'),
    tx.done
  ]);
  if (syncedAt !== undefined && !isTimestamp(syncedAt)) {
    throw new Error('설비 목록 갱신 시각 형식이 올바르지 않습니다.');
  }
  return { equipment: parseEquipmentList(equipment), syncedAt: syncedAt ?? null };
}

export async function replaceEquipment(equipment: Equipment[], syncedAt: number) {
  const db = await open();
  const tx = db.transaction(['equipment', 'meta'], 'readwrite');
  const store = tx.objectStore('equipment');
  await Promise.all([
    store.clear(),
    ...equipment.map((item) => store.put(item)),
    tx.objectStore('meta').put(syncedAt, 'equipmentSyncedAt'),
    tx.done
  ]);
}

export async function readRecords(authorId: UserId): Promise<LocalRecord[]> {
  const db = await open();
  const rows = await db.getAllFromIndex('records', 'byAuthor', authorId);
  return rows.map(parseLocalRecord).sort((a, b) => b.payload.inspectedAt - a.payload.inspectedAt);
}

export async function saveRecord(record: LocalRecord) {
  const db = await open();
  const tx = db.transaction('records', 'readwrite', { durability: 'strict' });
  await Promise.all([tx.store.add(record), tx.done]);
}

export async function saveCorrection(input: {
  authorId: UserId;
  rootId: InspectionId;
  basedOn: InspectionId;
  edit: InspectionEdit;
}) {
  const db = await open();
  const tx = db.transaction('records', 'readwrite', { durability: 'strict' });
  const stored = await tx.store.index('byAuthor').getAll(input.authorId);
  const [chain] = groupRecords(stored.map(parseLocalRecord).filter(({ payload }) => payload.rootId === input.rootId));
  if (!chain || chain.latest.id !== input.basedOn) {
    await tx.done;
    throw new Error('다른 화면에서 기록이 바뀌었습니다. 최신 내용을 확인한 뒤 다시 시도하세요.');
  }

  const record = createCorrection({ chain, id: crypto.randomUUID(), edit: input.edit });
  await Promise.all([tx.store.add(record), tx.done]);
  return record;
}

export async function finishPendingRecord(id: InspectionId, authorId: UserId, sync: FinishedSync) {
  const db = await open();
  const tx = db.transaction('records', 'readwrite');
  const value = await tx.store.get(id);
  const record = value === undefined ? undefined : parseLocalRecord(value);
  if (record?.authorId === authorId && record.sync.kind === 'pending') {
    const updated = { ...record, sync };
    await Promise.all([tx.store.put(updated), tx.done]);
    return updated;
  }
  await tx.done;
  return record?.authorId === authorId ? record : undefined;
}

export async function deleteAttentionRecord(id: InspectionId, authorId: UserId) {
  const db = await open();
  const tx = db.transaction('records', 'readwrite', { durability: 'strict' });
  const value = await tx.store.get(id);
  if (value !== undefined) {
    const record = parseLocalRecord(value);
    if (record.authorId !== authorId || record.sync.kind !== 'needs_attention') {
      await tx.done;
      return false;
    }
    await Promise.all([tx.store.delete(id), tx.done]);
  } else {
    await tx.done;
  }
  return true;
}
