import { isConflictReason, isObject, isReceivedAt, parseServerRecord } from './model';
import type { InspectionId, SyncState } from './model';

export type PutOutcome =
  | { kind: 'response'; status: number; body?: unknown }
  | { kind: 'network_error' }
  | { kind: 'timeout' };

export type FinishedSync = Exclude<SyncState, { kind: 'pending' }>;
type Transition =
  | { kind: 'continue'; sync: FinishedSync }
  | { kind: 'session_expired' }
  | { kind: 'retry' };

function classifyResponse(status: number, body: unknown, id: InspectionId): Transition {
  if ((status === 200 || status === 201) && isObject(body) && typeof body.id === 'string' &&
      body.id.toLowerCase() === id.toLowerCase() && isReceivedAt(body.receivedAt)) {
    return { kind: 'continue', sync: { kind: 'synced', receivedAt: body.receivedAt } };
  }
  if (status === 401) return { kind: 'session_expired' };
  if (status === 409 && isObject(body) && isConflictReason(body.reason)) {
    const sync: FinishedSync = { kind: 'needs_attention', reason: body.reason };
    if (body.current !== undefined) {
      try {
        sync.current = parseServerRecord(body.current);
      } catch {
        // 서버 기록을 읽을 수 없어도 충돌 사유는 표시할 수 있다.
      }
    }
    return { kind: 'continue', sync };
  }
  if (status === 429 || status < 400 || status >= 500) return { kind: 'retry' };
  return { kind: 'continue', sync: { kind: 'needs_attention', reason: status === 403 ? 'forbidden' : 'invalid' } };
}

export function classifyPut(outcome: PutOutcome, id: InspectionId): Transition {
  switch (outcome.kind) {
    case 'response':
      return classifyResponse(outcome.status, outcome.body, id);
    case 'network_error':
    case 'timeout':
      return { kind: 'retry' };
    default: {
      const exhaustive: never = outcome;
      return exhaustive;
    }
  }
}

export function retryDelay(attempt: number, random: number): number {
  return Math.min(60_000, 1000 * 2 ** attempt * (1 + random * 0.25));
}
