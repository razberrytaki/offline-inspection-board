<script lang="ts">
  import { onMount } from 'svelte';
  import { latestSynced, readFaults, replayInspection, setFaults, type FaultState } from './faults';
  import type { LocalRecord } from './model';

  let { records, sessionExpired, checkConnection }: {
    records: LocalRecord[];
    sessionExpired: boolean;
    checkConnection: () => void;
  } = $props();

  let faults = $state<FaultState | null>(null);
  let available = $derived(faults !== null);
  let open = $state(false);
  let busy = $state(false);
  let latest = $derived(latestSynced(records));
  let statuses = $state<(number | string)[]>([]);
  let error = $state('');
  const lifetime = new AbortController();
  const time = new Intl.DateTimeFormat('ko-KR', { timeStyle: 'medium' });

  function signalFor(controller: AbortController) {
    return AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]);
  }

  onMount(() => {
    void refresh(lifetime);
    return () => lifetime.abort();
  });

  async function refresh(controller: AbortController) {
    try {
      const state = await readFaults(signalFor(controller));
      if (state && !controller.signal.aborted) faults = state;
    } catch {
      // 실제 네트워크가 끊기면 마지막으로 확인한 상태를 유지한다.
    }
  }

  $effect(() => {
    if (!available || !open || busy) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      await refresh(controller);
      if (!controller.signal.aborted) timer = setTimeout(poll, 1000);
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  });

  async function run(action: (signal: AbortSignal) => Promise<void>) {
    if (busy) return;
    busy = true;
    error = '';
    try {
      await action(signalFor(lifetime));
    } catch {
      error = '요청에 실패했습니다. 장애 상태를 확인한 뒤 다시 시도하세요.';
    } finally {
      busy = false;
    }
  }

  function toggleConnection(offline: boolean) {
    void run(async (signal) => {
      await setFaults({ offline: !offline }, signal);
      checkConnection();
    });
  }

  function armDrop() {
    void run((signal) => setFaults({ dropAfterCommit: 1 }, signal));
  }

  function replay(record: LocalRecord) {
    void run(async (signal) => {
      statuses = [];
      statuses = await replayInspection(record, signal);
    });
  }
</script>

{#if faults}
  <details class="panel" bind:open>
    <summary>장애 패널</summary>
    <div class="controls">
      <button type="button" disabled={busy} onclick={() => faults && toggleConnection(faults.offline)}>
        {faults.offline ? '서버 연결 복구' : '서버 연결 끊기'}
      </button>
      <button type="button" disabled={busy || faults.dropAfterCommit > 0} onclick={armDrop}>
        저장 직후 응답 끊기
      </button>
      <button type="button" disabled={busy || faults.offline || sessionExpired || !latest}
        onclick={() => latest && replay(latest)}>
        같은 점검 5번 동시 전송
      </button>
    </div>
    {#if faults.dropAfterCommit > 0}
      <p role="status">응답 끊기 예약 {faults.dropAfterCommit}건</p>
    {/if}
    {#if statuses.length}
      <p role="status">동시 전송 결과 {statuses.join(' · ')}</p>
    {/if}
    {#if error}<p role="alert">{error}</p>{/if}
    <p>서버 점검 행 수 <strong>{faults.inspectionCount}</strong>건</p>

    <h2>최근 점검 요청</h2>
    {#if faults.log.length}
      <ul aria-label="최근 점검 요청">
        {#each faults.log as entry}
          <li>
            <time datetime={entry.at}>{time.format(new Date(entry.at))}</time>
            <code>{entry.id}</code>
            <strong>{entry.outcome}</strong>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="muted">아직 점검 요청이 없습니다.</p>
    {/if}
  </details>
{/if}

<style>
  details { margin-block-end: 1rem; }
  summary { cursor: pointer; font-weight: 650; }
  .controls { display: flex; flex-wrap: wrap; gap: .5rem; margin-block: 1rem .75rem; }
  button:disabled { opacity: .55; cursor: default; }
  p + p { margin-block-start: .35rem; }
  h2 { font-size: 1rem; margin-block: 1rem .35rem; }
  ul { margin: 0; padding-inline-start: 1.25rem; overflow-wrap: anywhere; }
  li + li { margin-block-start: .25rem; }
</style>
