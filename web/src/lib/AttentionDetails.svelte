<script lang="ts">
  import type { AttentionReason, EquipmentId, LocalRecord, SyncState } from './model';

  let { record, attention, equipmentNames, dateTime, canResubmit, resubmit, remove }:
    {
      record: LocalRecord;
      attention: Extract<SyncState, { kind: 'needs_attention' }>;
      equipmentNames: Map<EquipmentId, string>;
      dateTime: Intl.DateTimeFormat;
      canResubmit: boolean;
      resubmit: () => Promise<boolean>;
      remove: () => Promise<void>;
    } = $props();

  const reasons: Record<AttentionReason, string> = {
    content_mismatch: '같은 점검 ID에 다른 내용이 이미 접수되었습니다.',
    revision_taken: '같은 정정 차수가 이미 접수되었습니다.',
    missing_previous: '앞선 차수의 점검 기록이 서버에 없습니다.',
    forbidden: '이 점검을 정정할 권한이 없습니다.',
    invalid: '서버가 점검 내용을 처리할 수 없습니다.'
  };
  const id = $props.id();
  let expanded = $state(false);
  let resubmitting = $state(false);
  let deleting = $state(false);
  let confirmation: HTMLDialogElement;

  async function submitAgain() {
    resubmitting = true;
    try {
      if (await resubmit()) expanded = false;
    } finally {
      resubmitting = false;
    }
  }

  async function confirmDelete() {
    const result = confirmation.returnValue;
    confirmation.returnValue = '';
    if (result !== 'delete') return;
    deleting = true;
    try {
      await remove();
    } finally {
      deleting = false;
    }
  }
</script>

{#snippet version(title: string, payload: LocalRecord['payload'])}
  <div class="version">
    <h4>{title}</h4>
    <dl>
      <div><dt>설비</dt><dd>{payload.equipmentId} {equipmentNames.get(payload.equipmentId) ?? ''}</dd></div>
      <div><dt>결과</dt><dd>{payload.result === 'normal' ? '정상' : '이상'}</dd></div>
      <div><dt>점검 시각</dt><dd><time datetime={new Date(payload.inspectedAt).toISOString()}>{dateTime.format(payload.inspectedAt)}</time></dd></div>
      <div><dt>메모</dt><dd class="record-memo">{payload.memo || '메모 없음'}</dd></div>
    </dl>
  </div>
{/snippet}

<button type="button" class="record-detail" aria-expanded={expanded} aria-controls={id + '-details'}
  onclick={() => { expanded = !expanded; }}>
  확인 필요
</button>

<section id={id + '-details'} aria-labelledby={id + '-heading'} hidden={!expanded}>
  <h3 id={id + '-heading'}>확인 필요 상세</h3>
  <p>{reasons[attention.reason]}</p>
  <div class="versions" class:comparison={attention.current}>
    {@render version('기기 기록', record.payload)}
    {#if attention.current}
      {@render version('서버 기록', attention.current)}
    {/if}
  </div>
  <div class="actions">
    {#if canResubmit && record.payload.revision === 0}
      <button type="button" disabled={resubmitting || deleting} onclick={submitAgain}>새 점검으로 다시 제출</button>
    {/if}
    <button type="button" class="record-detail" disabled={resubmitting || deleting}
      onclick={() => confirmation.showModal()}>이 기기에서 삭제</button>
  </div>
</section>

<dialog bind:this={confirmation} aria-labelledby={id + '-delete-heading'} onclose={confirmDelete}>
  <h1 id={id + '-delete-heading'}>이 기기에서 점검 기록을 삭제할까요?</h1>
  <p>삭제한 기기 기록은 다시 열 수 없습니다.</p>
  <form method="dialog" class="actions">
    <!-- svelte-ignore a11y_autofocus (삭제 확인에서는 취소에 먼저 포커스를 둔다) -->
    <button value="cancel" class="record-detail" autofocus>취소</button>
    <button value="delete">삭제</button>
  </form>
</dialog>

<style>
  h3 { margin-block: .9rem .25rem; font-size: 1.05rem; }
  h4 { margin: 0 0 .4rem; }
  .versions { display: grid; gap: .5rem; margin-block: .75rem; }
  .comparison { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .version { background: #f2f5f2; border-radius: .45rem; padding: .55rem; }
  .version dl { display: grid; gap: .4rem; }
  .version dt { font-size: .875rem; color: #4f655a; }
  .actions { display: flex; flex-wrap: wrap; gap: .5rem; }
  .record-detail { color: #12372e; background: #fff; border-color: #739888; }
  dialog { max-inline-size: min(25rem, calc(100% - 4rem)); color: inherit; border: 1px solid #739888; border-radius: .8rem; padding: 1.25rem; }
  dialog h1 { font-size: 1.125rem; }
  dialog p { margin-block: .5rem 1rem; }
  dialog:modal { overscroll-behavior: contain; }
  dialog::backdrop { background: #162a2390; overflow: hidden; overscroll-behavior: contain; }
</style>
