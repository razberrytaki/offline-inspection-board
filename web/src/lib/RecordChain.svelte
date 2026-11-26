<script lang="ts">
  import { tick } from 'svelte';
  import AttentionDetails from './AttentionDetails.svelte';
  import CorrectionForm from './CorrectionForm.svelte';
  import { correctionBlock, type InspectionChain, type InspectionEdit } from './corrections';
  import type { EquipmentId, InspectionId, LocalRecord, SyncState } from './model';

  let { chain, equipmentNames, dateTime, sendingId, sessionExpired, correct, resubmit, remove }: {
    chain: InspectionChain;
    equipmentNames: Map<EquipmentId, string>;
    dateTime: Intl.DateTimeFormat;
    sendingId: InspectionId | null;
    sessionExpired: boolean;
    correct: (record: LocalRecord, edit: InspectionEdit) => Promise<boolean>;
    resubmit: (record: LocalRecord) => Promise<boolean>;
    remove: (record: LocalRecord) => Promise<void>;
  } = $props();

  const syncLabels: Record<SyncState['kind'], string> = {
    pending: '전송 대기', synced: '동기화 완료', needs_attention: '확인 필요'
  };
  let showHistory = $state(false);
  let editing = $state(false);
  let editButton = $state<HTMLButtonElement>();
  let blocked = $derived(correctionBlock(chain));
  let { equipmentId, inspectedAt, memo, result } = $derived(chain.latest.payload);

  async function closeEditor() {
    editing = false;
    await tick();
    editButton?.focus();
  }
</script>

<div class="record-heading">
  <strong>{equipmentId} {equipmentNames.get(equipmentId) ?? ''}</strong>
  {#if chain.state !== 'needs_attention'}
    <span class="record-state" aria-live="polite">{sendingId && chain.history.some(({ id }) => id === sendingId) ? '전송 중' : syncLabels[chain.state]}</span>
  {/if}
</div>
<p>{result === 'normal' ? '정상' : '이상'} · <time datetime={new Date(inspectedAt).toISOString()}>{dateTime.format(inspectedAt)}</time></p>
{#if memo}<p class="record-memo">{memo}</p>{/if}

<details bind:open={showHistory}>
  <summary>점검 이력 ({chain.history.length}건)</summary>
  {#if showHistory}
    <ol aria-label="점검 차수">
      {#each chain.history as record (record.id)}
        <li>
          <div class="record-heading">
            <strong>{record.payload.revision === 0 ? '원본' : '정정 ' + record.payload.revision}</strong>
            <span class="record-state">{record.id === sendingId && record.sync.kind === 'pending' ? '전송 중' : syncLabels[record.sync.kind]}</span>
          </div>
          <p>{record.payload.result === 'normal' ? '정상' : '이상'}</p>
          <p class="record-memo">{record.payload.memo || '메모 없음'}</p>
        </li>
      {/each}
    </ol>
  {/if}
</details>

{#each chain.history as record (record.id)}
  {#if record.sync.kind === 'needs_attention'}
    <div class="attention-details">
      {#if chain.history.length > 1}
        <span>{record.payload.revision === 0 ? '원본' : '정정 ' + record.payload.revision}</span>
      {/if}
      <AttentionDetails {record} attention={record.sync} {equipmentNames} {dateTime}
        canResubmit={!sessionExpired} resubmit={() => resubmit(record)} remove={() => remove(record)} />
    </div>
  {/if}
{/each}

{#if blocked}
  <p class="muted">{blocked}</p>
{:else if !sessionExpired}
  {#if editing}
    {#key chain.latest.id}
      {@const latest = chain.latest}
      <CorrectionForm latest={latest.payload} save={(edit) => correct(latest, edit)} cancel={closeEditor} />
    {/key}
  {:else}
    <button type="button" class="edit" bind:this={editButton} onclick={() => { editing = true; }}>정정하기</button>
  {/if}
{/if}

<style>
  details { margin-block: .65rem; }
  summary { cursor: pointer; color: #1d614d; }
  ol { list-style: none; margin: .45rem 0 0; padding-inline-start: .75rem; }
  .edit { color: #12372e; background: #fff; border-color: #739888; }
</style>
