import { applyRow, parseBoardList, parseRow, type BoardRows, type Row } from './board';
import { parseUser } from './model';

export type BoardConnection = 'connecting' | 'live' | 'login';

export function connectBoard(view: {
  connection: (state: BoardConnection) => void;
  rows: (rows: BoardRows, changed: Row[]) => void;
}) {
  let source: EventSource | undefined;
  let snapshot: { controller: AbortController; buffer: Row[] } | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let loginRequired = false;
  let rows: BoardRows | undefined;

  function stopSnapshot() {
    snapshot?.controller.abort();
    snapshot = undefined;
  }

  function stop() {
    stopSnapshot();
    source?.close();
    source = undefined;
    clearTimeout(retry);
  }

  function requireLogin() {
    loginRequired = true;
    rows = undefined;
    stop();
    view.rows(new Map(), []);
    view.connection('login');
  }

  function reconnect() {
    stop();
    view.connection('connecting');
    retry = setTimeout(start, 1000);
  }

  async function checkClosedStream(events: EventSource) {
    try {
      const response = await fetch('/api/me', { cache: 'no-store', signal: AbortSignal.timeout(8000) });
      if (source !== events) return;
      if (response.status === 401 || response.status === 403) {
        requireLogin();
        return;
      }
      if (response.ok) {
        const user = parseUser(await response.json());
        if (source !== events) return;
        if (user.role !== 'supervisor') {
          requireLogin();
          return;
        }
      }
    } catch {
      // 연결이 끊겨 계정 확인도 못 하면 스트림을 다시 연다.
    }
    if (source === events) reconnect();
  }

  async function refresh(events: EventSource) {
    stopSnapshot();
    const reading = { controller: new AbortController(), buffer: new Array<Row>() };
    snapshot = reading;
    try {
      const response = await fetch('/api/board', {
        cache: 'no-store',
        signal: AbortSignal.any([reading.controller.signal, AbortSignal.timeout(8000)])
      });
      if (snapshot !== reading) return;
      if (response.status === 401 || response.status === 403) {
        requireLogin();
        return;
      }
      if (!response.ok) throw new Error('보드 목록을 불러오지 못했습니다.');
      const listed = parseBoardList(await response.json());
      if (snapshot !== reading || events.readyState !== EventSource.OPEN) return;

      const merged = reading.buffer.reduce(applyRow, listed);
      const bufferedIds = new Set(reading.buffer.map(({ rootId }) => rootId));
      const changed = [...merged.values()].filter((row) => rows
        ? row.revision > (rows.get(row.rootId)?.revision ?? -1)
        : bufferedIds.has(row.rootId)
      );
      snapshot = undefined;
      rows = merged;
      view.rows(rows, changed);
      view.connection('live');
    } catch {
      if (snapshot === reading) reconnect();
    }
  }

  function start() {
    clearTimeout(retry);
    if (source || loginRequired || !navigator.onLine) return;
    const events = new EventSource('/api/board/events');
    source = events;
    events.onopen = () => {
      if (source === events) void refresh(events);
    };
    events.onerror = () => {
      if (source !== events) return;
      stopSnapshot();
      view.connection('connecting');
      if (events.readyState === EventSource.CLOSED) void checkClosedStream(events);
    };
    events.addEventListener('row', (event) => {
      if (source !== events || !(event instanceof MessageEvent) || typeof event.data !== 'string') return;
      let row: Row;
      try {
        row = parseRow(JSON.parse(event.data));
      } catch {
        return;
      }
      if (snapshot) {
        snapshot.buffer.push(row);
      } else if (rows) {
        const next = applyRow(rows, row);
        if (next === rows) return;
        rows = next;
        view.rows(rows, [row]);
      }
    });
  }

  function offline() {
    stop();
    if (!loginRequired) view.connection('connecting');
  }

  window.addEventListener('online', start);
  window.addEventListener('offline', offline);
  start();
  return () => {
    stop();
    window.removeEventListener('online', start);
    window.removeEventListener('offline', offline);
  };
}
