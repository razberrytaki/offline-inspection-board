<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import BoardLoginPrompt from '$lib/BoardLoginPrompt.svelte';
  import { boardDateTime, visibleRows, type BoardFilter, type BoardRows, type Row } from '$lib/board';
  import { connectBoard, type BoardConnection } from '$lib/board-stream';
  import { beginLogout } from '$lib/storage';

  let connection = $state<BoardConnection>('connecting');
  let rows = $state.raw<BoardRows>(new Map());
  let hasList = $state(false);
  let filter = $state<BoardFilter>('all');
  let displayed = $derived(visibleRows(rows, filter));
  let signingOut = $state(false);
  let error = $state('');
  const highlighted = new SvelteSet<Row>();
  const timers = new Set<ReturnType<typeof setTimeout>>();

  onMount(() => {
    const disconnect = connectBoard({
      connection: (state) => { connection = state; },
      rows: (next, changed) => {
        rows = next;
        hasList = true;
        for (const row of changed) {
          highlighted.add(row);
          const timer = setTimeout(() => {
            highlighted.delete(row);
            timers.delete(timer);
          }, 1200);
          timers.add(timer);
        }
      }
    });

    return () => {
      disconnect();
      for (const timer of timers) clearTimeout(timer);
    };
  });

  async function logout() {
    signingOut = true;
    error = '';
    try {
      await beginLogout();
      await goto('/', { replaceState: true });
    } catch {
      error = '로그아웃을 시작하지 못했습니다. 다시 시도하세요.';
    } finally {
      signingOut = false;
    }
  }
</script>

<svelte:head>
  <title>감독자 보드</title>
  <meta name="theme-color" content="#12372e" />
</svelte:head>

<header class="masthead">
  <div class="container masthead-row">
    <h1>감독자 보드</h1>
    {#if hasList && connection !== 'login'}
      <button type="button" class="secondary" disabled={signingOut} onclick={logout}>로그아웃</button>
    {/if}
  </div>
</header>

<main class="container">
  {#if connection === 'login'}
    <BoardLoginPrompt />
  {:else}
    {#if error}<p class="notice error-notice" role="alert">{error}</p>{/if}
    <section class="panel" aria-label="접수된 점검">
      <div class="toolbar">
        <p role="status" class="connection" class:live={connection === 'live'}>
          {connection === 'live' ? '실시간 연결됨' : '다시 연결 중'}
        </p>
        <fieldset>
          <legend class="visually-hidden">현재 결과 필터</legend>
          <label><input type="radio" name="filter" value="all" bind:group={filter} />전체</label>
          <label><input type="radio" name="filter" value="abnormal" bind:group={filter} />이상</label>
        </fieldset>
      </div>
      {#if hasList}
        <!-- svelte-ignore a11y_no_noninteractive_tabindex (키보드로 표를 가로로 스크롤할 수 있게 한다) -->
        <div class="table-scroll" role="region" tabindex="0" aria-labelledby="board-caption">
          <table>
            <caption id="board-caption">서버 접수 점검</caption>
            <thead>
              <tr>
                <th scope="col">설비</th>
                <th scope="col">현재 결과</th>
                <th scope="col">메모</th>
                <th scope="col">작성자</th>
                <th scope="col">점검 시각</th>
                <th scope="col">최근 접수 시각</th>
              </tr>
            </thead>
            <tbody>
              {#each displayed as row (row.rootId)}
                <tr data-root-id={row.rootId} data-revision={row.revision} class:highlighted={highlighted.has(row)}>
                  <th scope="row"><a href={'/board/' + row.rootId}>{row.equipmentId} {row.equipmentName}</a></th>
                  <td>
                    <span class="result" class:abnormal={row.result === 'abnormal'}>{row.result === 'abnormal' ? '이상' : '정상'}</span>
                    {#if row.revision > 0}<span class="corrected">정정됨</span>{/if}
                  </td>
                  <td class="record-memo">{row.memo || '—'}</td>
                  <td>{row.authorName}</td>
                  <td><time datetime={new Date(row.inspectedAt).toISOString()}>{boardDateTime.format(row.inspectedAt)}</time></td>
                  <td><time datetime={row.receivedAt}>{boardDateTime.format(new Date(row.receivedAt))}</time></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if displayed.length === 0}
          <p class="empty muted">{filter === 'all' ? '접수된 점검이 없습니다.' : '현재 이상인 점검이 없습니다.'}</p>
        {/if}
      {/if}
    </section>
  {/if}
</main>

<style>
  .toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: .75rem; }
  .connection { color: #673f00; font-weight: 650; }
  .connection.live { color: #1d614d; }
  fieldset { display: flex; gap: .35rem; }
  label { display: flex; align-items: center; gap: .3rem; padding: .45rem .65rem; border-radius: .45rem; border: 1px solid #c8d6cd; cursor: pointer; }
  label:has(:checked) { border-color: #1d614d; background: #e8f3ed; }
  .table-scroll { overflow: auto; max-block-size: min(34rem, 70vh); margin-block-start: 1rem; }
  table { width: 100%; border-collapse: separate; border-spacing: 0; }
  caption { text-align: start; font-weight: 650; padding-block-end: .5rem; }
  th, td { text-align: start; vertical-align: top; padding: .75rem; border-block-end: 1px solid #c8d6cd; }
  thead { position: sticky; inset-block-start: 0; z-index: 1; background: #f2f5f2; white-space: nowrap; }
  tbody th { position: sticky; inset-inline-start: 0; background: #fff; min-inline-size: 7rem; }
  tbody td { background: #fff; }
  tbody td.record-memo { min-inline-size: 10rem; overflow-wrap: anywhere; }
  a { color: #1d614d; }
  .result { display: inline-block; font-weight: 650; }
  .abnormal { color: #9a2b26; }
  .corrected { display: block; color: #4f655a; font-size: .85rem; white-space: nowrap; }
  .highlighted > :is(th, td) { background-color: #fff2d6; }
  @media (prefers-reduced-motion: no-preference) {
    tbody :is(th, td) { transition: background-color 400ms ease-out; }
    .highlighted > :is(th, td) { transition: none; }
  }
  .empty { padding-block-start: .75rem; }
</style>
