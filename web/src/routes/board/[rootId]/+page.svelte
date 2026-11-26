<script lang="ts">
  import BoardLoginPrompt from '$lib/BoardLoginPrompt.svelte';
  import { boardDateTime, type Row } from '$lib/board';
  import type { PageProps } from './$types';

  let { data }: PageProps = $props();
</script>

<svelte:head>
  <title>점검 상세 · 감독자 보드</title>
  <meta name="theme-color" content="#12372e" />
</svelte:head>

<header class="masthead">
  <div class="container masthead-row">
    <h1>점검 상세</h1>
    <a href="/board">보드로 돌아가기</a>
  </div>
</header>

{#snippet values(row: Row)}
  <div><dt>결과</dt><dd>{row.result === 'abnormal' ? '이상' : '정상'}</dd></div>
  <div><dt>메모</dt><dd class="record-memo">{row.memo || '메모 없음'}</dd></div>
  <div><dt>접수 시각</dt><dd><time datetime={row.receivedAt}>{boardDateTime.format(new Date(row.receivedAt))}</time></dd></div>
{/snippet}

<main class="container">
  {#if data.kind === 'login'}
    <BoardLoginPrompt />
  {:else if data.kind === 'error'}
    <p class="notice error-notice" role="alert">{data.message}</p>
  {:else}
    {@const { current, history } = data.detail}
    <section class="panel" aria-labelledby="current-inspection">
      <h2 id="current-inspection">현재 기록</h2>
      <dl class="current">
        <div><dt>설비</dt><dd>{current.equipmentId} {current.equipmentName}</dd></div>
        <div><dt>작성자</dt><dd>{current.authorName}</dd></div>
        <div><dt>점검 시각</dt><dd><time datetime={new Date(current.inspectedAt).toISOString()}>{boardDateTime.format(current.inspectedAt)}</time></dd></div>
        {@render values(current)}
      </dl>
    </section>
    <section class="panel" aria-labelledby="inspection-history">
      <h2 id="inspection-history">점검 이력</h2>
      <ol class="records">
        {#each history as row (row.id)}
          <li>
            <h3>{row.revision === 0 ? '원본' : '정정 ' + row.revision}</h3>
            <dl>{@render values(row)}</dl>
          </li>
        {/each}
      </ol>
    </section>
  {/if}
</main>

<style>
  a { color: inherit; }
  .panel + .panel { margin-block-start: 1rem; }
  h3 { margin: 0 0 .5rem; font-size: 1rem; }
  dl { display: grid; gap: .6rem; overflow-wrap: anywhere; }
  .current { grid-template-columns: repeat(auto-fit, minmax(min(100%, 14rem), 1fr)); }
  dt { color: #4f655a; font-size: .875rem; }
</style>
