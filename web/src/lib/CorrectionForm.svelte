<script lang="ts">
  import { hasInspectionChanges, type InspectionEdit } from './corrections';

  let { latest, save, cancel }: {
    latest: InspectionEdit;
    save: (edit: InspectionEdit) => Promise<boolean>;
    cancel: () => void;
  } = $props();

  const id = $props.id();
  const memoError = '오류: 이상 결과에는 메모를 입력하세요.';
  let result = $state<InspectionEdit['result']>('normal');
  let memo = $state('');
  let memoField = $state<HTMLTextAreaElement>();
  let noChange = $state(false);
  let saving = $state(false);

  $effect(() => {
    result = latest.result;
    memo = latest.memo;
  });
  $effect(() => {
    memoField?.setCustomValidity(result === 'abnormal' && memo.length > 0 && !memo.trim() ? memoError : '');
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (saving) return;
    const edit = { result, memo };
    noChange = !hasInspectionChanges(edit, latest);
    if (noChange) return;

    saving = true;
    try {
      if (await save(edit)) cancel();
    } finally {
      saving = false;
    }
  }
</script>

<form aria-label="점검 정정" onsubmit={submit} oninput={() => { noChange = false; }}>
  <fieldset>
    <legend>정정 결과</legend>
    <div class="choices results">
      <label class="choice"><input type="radio" name="result" value="normal" bind:group={result} />정상</label>
      <label class="choice"><input type="radio" name="result" value="abnormal" bind:group={result} />이상</label>
    </div>
  </fieldset>
  <div>
    <label for={id + '-memo'}>정정 메모{result === 'abnormal' ? ' (필수)' : ''}</label>
    <textarea id={id + '-memo'} rows="3" required={result === 'abnormal'}
      bind:this={memoField} bind:value={memo} data-error={id + '-error'} aria-invalid="false"></textarea>
    <p class="field-error" id={id + '-error'}>{memoError}</p>
  </div>
  {#if noChange}<p class="notice error-notice" role="alert">결과나 메모를 변경하세요.</p>{/if}
  <div class="actions">
    <button type="submit" disabled={saving}>정정 저장</button>
    <button type="button" disabled={saving} onclick={cancel}>취소</button>
  </div>
</form>

<style>
  form { margin-block-start: .75rem; }
  .actions { display: flex; flex-wrap: wrap; gap: .5rem; }
</style>
