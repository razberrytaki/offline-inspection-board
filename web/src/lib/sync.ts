import type { InspectionId, LocalRecord, UserId } from './model';
import { finishPendingRecord, readRecords, readSession } from './storage';
import { classifyPut, retryDelay, type PutOutcome } from './sync-outcome';

export function pendingInSendOrder(records: LocalRecord[]): LocalRecord[] {
  return records.filter(({ sync }) => sync.kind === 'pending').sort((a, b) =>
    a.payload.inspectedAt - b.payload.inspectedAt ||
    a.payload.rootId.localeCompare(b.payload.rootId) ||
    a.payload.revision - b.payload.revision
  );
}

async function put(record: LocalRecord, signal: AbortSignal): Promise<PutOutcome> {
  const timeout = AbortSignal.timeout(8000);
  try {
    const response = await fetch('/api/inspections/' + record.id, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(record.payload),
      signal: AbortSignal.any([signal, timeout]),
      cache: 'no-store'
    });
    if (response.ok || response.status === 409) {
      const text = await response.text();
      try {
        const body: unknown = JSON.parse(text);
        return { kind: 'response', status: response.status, body };
      } catch {
        return { kind: 'response', status: response.status };
      }
    }
    return { kind: 'response', status: response.status };
  } catch {
    return { kind: timeout.aborted ? 'timeout' : 'network_error' };
  }
}

export function createSync(view: {
  sending: (id: InspectionId | null) => void;
  connection: (state: 'online' | 'offline') => void;
  records: (records: LocalRecord[]) => void;
  updated: (record: LocalRecord) => void;
  expired: () => void;
  error: () => void;
}) {
  let session: { authorId: UserId; controller: AbortController } | null = null;
  let running = false;
  let queued = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let attempt = 0;

  function setAuthor(authorId: UserId | null) {
    if ((session?.authorId ?? null) === authorId) return;
    session?.controller.abort();
    clearTimeout(timer);
    timer = undefined;
    attempt = 0;
    queued = false;
    view.sending(null);
    session = authorId === null ? null : { authorId, controller: new AbortController() };
  }

  function scheduleRetry() {
    timer = setTimeout(() => {
      timer = undefined;
      trigger();
    }, retryDelay(attempt, Math.random()));
    attempt = Math.min(attempt + 1, 6);
  }

  async function drain(authorId: UserId, signal: AbortSignal) {
    const records = await readRecords(authorId);
    if (signal.aborted) return;
    view.records(records);

    const pending = pendingInSendOrder(records);

    for (const record of pending) {
      const cached = await readSession();
      if (signal.aborted) return;
      if (cached.logoutPending || cached.user?.userId !== authorId) {
        view.expired();
        return;
      }

      view.sending(record.id);
      const outcome = await put(record, signal);
      if (signal.aborted) return;

      view.connection(outcome.kind === 'response' ? 'online' : 'offline');
      const transition = classifyPut(outcome, record.id);
      switch (transition.kind) {
        case 'continue': {
          if (transition.sync.kind === 'synced') attempt = 0;
          const updated = await finishPendingRecord(record.id, authorId, transition.sync);
          if (signal.aborted) return;
          if (updated) view.updated(updated);
          view.sending(null);
          break;
        }
        case 'session_expired':
          view.expired();
          return;
        case 'retry':
          scheduleRetry();
          return;
        default: {
          const exhaustive: never = transition;
          return exhaustive;
        }
      }
    }
  }

  async function run({ authorId, controller: { signal } }: NonNullable<typeof session>) {
    try {
      if (navigator.locks) {
        await navigator.locks.request('oib-sync', { ifAvailable: true }, (lock) => {
          if (signal.aborted) return;
          if (lock) return drain(authorId, signal);
          scheduleRetry();
        });
      } else {
        await drain(authorId, signal);
      }
    } catch {
      if (!signal.aborted) {
        view.error();
        scheduleRetry();
      }
    } finally {
      view.sending(null);
      running = false;
      if (queued) trigger();
    }
  }

  function trigger({ wake = false } = {}) {
    if (!session) return;
    if (wake) {
      clearTimeout(timer);
      timer = undefined;
    }
    if (running || timer !== undefined) {
      queued = true;
      return;
    }
    queued = false;
    running = true;
    void run(session);
  }

  return { setAuthor, trigger };
}
