import {
  isInspectionId, isObject, isReceivedAt, isText, isUserId, parsePayload, type InspectionId, type ServerRecord
} from './model';

export type Row = Omit<ServerRecord, 'id'> & { equipmentName: string; authorName: string };
export type BoardRows = ReadonlyMap<InspectionId, Row>;
export type BoardFilter = 'all' | 'abnormal';

export function parseRow(value: unknown): Row {
  if (!isObject(value) || !isText(value.equipmentName) || !isText(value.authorName) ||
      !isUserId(value.authorId) || !isReceivedAt(value.receivedAt)) {
    throw new Error('보드 기록 형식이 올바르지 않습니다.');
  }
  return {
    ...parsePayload(value),
    equipmentName: value.equipmentName,
    authorId: value.authorId,
    authorName: value.authorName,
    receivedAt: value.receivedAt
  };
}

export function parseBoardList(value: unknown): BoardRows {
  if (!Array.isArray(value)) throw new Error('보드 목록 형식이 올바르지 않습니다.');
  const rows = new Map<InspectionId, Row>();
  for (const item of value) {
    const row = parseRow(item);
    if (row.revision > (rows.get(row.rootId)?.revision ?? -1)) rows.set(row.rootId, row);
  }
  return rows;
}

export function applyRow(rows: BoardRows, row: Row): BoardRows {
  if (row.revision <= (rows.get(row.rootId)?.revision ?? -1)) return rows;
  return new Map(rows).set(row.rootId, row);
}

function fractionalSecond(time: string) {
  return Number('0.' + (/\.(\d+)/.exec(time)?.[1] ?? '0'));
}

export function visibleRows(rows: BoardRows, filter: BoardFilter): Row[] {
  return [...rows.values()].filter((row) => filter === 'all' || row.result === 'abnormal')
    .sort((a, b) => Date.parse(b.receivedAt) - Date.parse(a.receivedAt)
      || fractionalSecond(b.receivedAt) - fractionalSecond(a.receivedAt)
      || a.rootId.localeCompare(b.rootId));
}

export function parseBoardDetail(value: unknown, rootId: string) {
  if (!isObject(value) || !Array.isArray(value.history)) throw new Error('점검 이력 형식이 올바르지 않습니다.');
  const current = parseRow(value.current);
  const history = Array.from(value.history, (entry: unknown, index) => {
    const row = parseRow(entry);
    if (!isObject(entry) || !isInspectionId(entry.id) || row.rootId !== current.rootId ||
        row.revision !== index || (index === 0) !== (entry.id === row.rootId)) {
      throw new Error('점검 이력 형식이 올바르지 않습니다.');
    }
    return { ...row, id: entry.id };
  });
  if (current.rootId !== rootId.toLowerCase() || history.length !== current.revision + 1) {
    throw new Error('점검 이력 형식이 올바르지 않습니다.');
  }
  return { current, history };
}

export const boardDateTime = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'medium' });
