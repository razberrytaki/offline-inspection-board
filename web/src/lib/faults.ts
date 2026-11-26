import { isObject, isReceivedAt, type LocalRecord } from './model';

type FaultAttempt = {
  at: string;
  id: string;
  outcome: number | 'dropped' | 'offline';
};

export type FaultState = {
  dropAfterCommit: number;
  offline: boolean;
  inspectionCount: number;
  log: FaultAttempt[];
};

const endpoint = '/api/dev/faults';

function isCount(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
}

function parseState(value: unknown): FaultState {
  if (!isObject(value)) throw new Error('장애 상태 형식이 올바르지 않습니다.');
  const { dropAfterCommit, offline, inspectionCount, log } = value;
  if (!isCount(dropAfterCommit) || typeof offline !== 'boolean' || !isCount(inspectionCount) ||
      !Array.isArray(log) || log.length > 20) {
    throw new Error('장애 상태 형식이 올바르지 않습니다.');
  }
  return {
    dropAfterCommit, offline, inspectionCount,
    log: Array.from(log, (entry: unknown) => {
      if (!isObject(entry)) throw new Error('점검 요청 로그 형식이 올바르지 않습니다.');
      const { at, id, outcome } = entry;
      if (!isReceivedAt(at) || typeof id !== 'string' ||
          (outcome !== 'dropped' && outcome !== 'offline' &&
            (!isCount(outcome) || outcome < 100 || outcome > 599))) {
        throw new Error('점검 요청 로그 형식이 올바르지 않습니다.');
      }
      return { at, id, outcome };
    })
  };
}

export async function readFaults(signal: AbortSignal): Promise<FaultState | null> {
  const response = await fetch(endpoint, { cache: 'no-store', signal });
  if (response.status !== 200) return null;
  const value: unknown = await response.json();
  return parseState(value);
}

export async function setFaults(settings: { offline: boolean } | { dropAfterCommit: number }, signal: AbortSignal) {
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
    signal
  });
  if (response.status !== 204) throw new Error('장애 설정을 바꾸지 못했습니다.');
}

export function latestSynced(records: LocalRecord[]) {
  let latest: { record: LocalRecord; receivedAt: number } | undefined;
  for (const record of records) {
    if (record.sync.kind !== 'synced') continue;
    const receivedAt = Date.parse(record.sync.receivedAt);
    if (!latest || receivedAt > latest.receivedAt ||
        (receivedAt === latest.receivedAt && record.payload.revision > latest.record.payload.revision)) {
      latest = { record, receivedAt };
    }
  }
  return latest?.record;
}

export function replayInspection(record: LocalRecord, signal: AbortSignal) {
  const body = JSON.stringify(record.payload);
  return Promise.all(Array.from({ length: 5 }, async () => {
    try {
      const response = await fetch('/api/inspections/' + record.id, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body,
        cache: 'no-store',
        signal
      });
      return response.status;
    } catch {
      return '연결 끊김';
    }
  }));
}
