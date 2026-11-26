import { parseLocalRecord, type InspectionId, type LocalRecord, type SyncState } from './model';

export type InspectionEdit = Pick<LocalRecord['payload'], 'result' | 'memo'>;
export type InspectionChain = {
  latest: LocalRecord;
  history: LocalRecord[];
  state: SyncState['kind'];
};

export function groupRecords(records: LocalRecord[]): InspectionChain[] {
  const groups = new Map<InspectionId, InspectionChain>();
  for (const record of records) {
    const { rootId, revision } = record.payload;
    const chain = groups.get(rootId);
    if (!chain) {
      groups.set(rootId, { latest: record, history: [record], state: record.sync.kind });
      continue;
    }
    chain.history.push(record);
    if (revision > chain.latest.payload.revision) chain.latest = record;
    if (record.sync.kind === 'needs_attention' || chain.state === 'synced') {
      chain.state = record.sync.kind;
    }
  }
  return [...groups.values()].map((chain) => {
    chain.history.sort((a, b) => a.payload.revision - b.payload.revision);
    return chain;
  });
}

export function nextRevision(chain: InspectionChain) {
  return chain.latest.payload.revision + 1;
}

export function correctionBlock(chain: InspectionChain) {
  if (chain.state === 'needs_attention') return '확인 필요 기록을 해결한 뒤 정정할 수 있습니다.';
  if (chain.history.some(({ payload }, index) => payload.revision !== index)) {
    return '기기에 원본 또는 앞선 정정 기록이 없어 정정할 수 없습니다.';
  }
  return '';
}

export function hasInspectionChanges(edit: InspectionEdit, latest: InspectionEdit) {
  return edit.result !== latest.result || edit.memo.trim().normalize('NFC') !== latest.memo.trim().normalize('NFC');
}

export function createCorrection({ chain, id, edit }: { chain: InspectionChain; id: string; edit: InspectionEdit }) {
  const blocked = correctionBlock(chain);
  if (blocked) throw new Error(blocked);
  if (!hasInspectionChanges(edit, chain.latest.payload)) {
    throw new Error('결과나 메모를 변경하세요.');
  }

  const original = chain.history[0];
  return parseLocalRecord({
    id,
    authorId: original.authorId,
    payload: { ...original.payload, ...edit, revision: nextRevision(chain) },
    sync: { kind: 'pending' }
  });
}
