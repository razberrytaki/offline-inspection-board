<script lang="ts">
  import { isEquipmentId, isResult, searchEquipment, type Equipment, type Result } from './model';

  let { equipment, save }: {
    equipment: Equipment[];
    save: (input: { equipmentId: Equipment['id']; result: Result; memo: string }) => Promise<boolean>;
  } = $props();

  const memoError = '오류: 이상 결과에는 메모를 입력하세요.';
  let query = $state('');
  let result = $state<Result>();
  let memo = $state('');
  let memoField = $state<HTMLTextAreaElement>();
  let saving = $state(false);
  let visibleEquipment = $derived(searchEquipment(equipment, query));
  let searchField = $state<HTMLInputElement>();

  $effect(() => {
    searchField?.setCustomValidity(visibleEquipment.length ? '' : '저장된 설비를 선택할 수 있도록 검색어를 확인하세요.');
    memoField?.setCustomValidity(result === 'abnormal' && memo.length > 0 && !memo.trim() ? memoError : '');
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (saving || !(event.currentTarget instanceof HTMLFormElement)) return;

    const form = event.currentTarget;
    const fields = new FormData(form);
    const equipmentId = fields.get('equipmentId');
    if (!isEquipmentId(equipmentId) || !isResult(result)) return;

    saving = true;
    try {
      if (await save({ equipmentId, result, memo })) form.reset();
    } finally {
      saving = false;
    }
  }
</script>

<section class="panel" aria-labelledby="inspection-heading">
  <h2 id="inspection-heading">점검 기록하기</h2>
  <form onsubmit={submit}>
    <div>
      <search>
        <label for="equipment-search">설비 검색</label>
        <input id="equipment-search" type="search" name="equipmentSearch" bind:this={searchField}
          bind:value={query} placeholder="설비 번호 또는 이름" spellcheck="false"
          data-error="search-error" aria-invalid="false" />
        <p class="field-error" id="search-error">오류: 선택할 설비가 없습니다.</p>
      </search>
      <fieldset role="radiogroup" aria-labelledby="equipment-legend" aria-invalid="false" data-error="equipment-error">
        <legend id="equipment-legend">설비 선택 (필수)</legend>
        {#if visibleEquipment.length}
          <div class="choices">
            {#each visibleEquipment as item (item.id)}
              <label class="choice">
                <input type="radio" name="equipmentId" value={item.id} required data-error="equipment-error" />
                <span>{item.id} {item.name}</span>
              </label>
            {/each}
          </div>
        {:else}
          <p class="muted">{equipment.length ? '검색 결과가 없습니다.' : '기기에 저장된 설비가 없습니다.'}</p>
        {/if}
        <p class="field-error" id="equipment-error">오류: 설비를 선택하세요.</p>
      </fieldset>
    </div>

    <fieldset role="radiogroup" aria-labelledby="result-legend" aria-invalid="false" data-error="result-error">
      <legend id="result-legend">점검 결과 (필수)</legend>
      <div class="choices results">
        <label class="choice">
          <input type="radio" name="result" value="normal" bind:group={result} required
            data-error="result-error" />
          정상
        </label>
        <label class="choice">
          <input type="radio" name="result" value="abnormal" bind:group={result} required
            data-error="result-error" />
          이상
        </label>
      </div>
      <p class="field-error" id="result-error">오류: 점검 결과를 선택하세요.</p>
    </fieldset>

    <div>
      <label for="memo">메모{result === 'abnormal' ? ' (필수)' : ''}</label>
      <textarea id="memo" name="memo" rows="3" required={result === 'abnormal'}
        bind:this={memoField} bind:value={memo} data-error="memo-error" aria-invalid="false"></textarea>
      <p class="field-error" id="memo-error">{memoError}</p>
    </div>

    <button type="submit" aria-disabled={saving}>{saving ? '저장 중…' : '기기에 저장'}</button>
  </form>
</section>
