<script lang="ts">
  import { onMount } from 'svelte';
  import InspectionForm from '$lib/InspectionForm.svelte';
  import {
    createRecord, parseEquipmentList, parseUser,
    type Equipment, type EquipmentId, type LocalRecord, type Result, type User
  } from '$lib/model';
  import {
    beginLogout, cacheUser, completeLogout, readEquipment, readRecords, readSession,
    replaceEquipment, saveRecord
  } from '$lib/storage';
  import { observeValidation } from '$lib/validation';
  import './app.css';

  let user = $state<User | null>(null);
  let equipment = $state<Equipment[]>([]);
  let syncedAt = $state<number | null>(null);
  let records = $state<LocalRecord[]>([]);
  let connection = $state<'checking' | 'online' | 'offline'>('checking');
  let busy = $state(false);
  let opening = $state(true);
  let retryQueued = false;

  let persistenceWarning = $state(false);
  let error = $state('');
  let saved = $state(false);
  let validationMessage = $state('');
  let pendingCount = $derived(records.filter((item) => item.sync.kind === 'pending').length);
  let equipmentNames = $derived(new Map(equipment.map(({ id, name }) => [id, name])));
  const dateTime = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' });
  const syncLabels: Record<LocalRecord['sync']['kind'], string> = {
    pending: '전송 대기',
    synced: '동기화 완료',
    needs_attention: '확인 필요',
    needs_login: '다시 로그인 필요'
  };
  const connectionLabels = { checking: '확인 중', online: '온라인', offline: '오프라인' };

  onMount(() => {
    const stopValidation = observeValidation((message) => { validationMessage = message; });
    retryConnection();
    void checkPersistence();
    window.addEventListener('online', retryConnection);
    window.addEventListener('offline', retryConnection);
    return () => {
      stopValidation();
      window.removeEventListener('online', retryConnection);
      window.removeEventListener('offline', retryConnection);
    };
  });

  async function checkPersistence() {
    try {
      persistenceWarning = !(await navigator.storage.persisted() || await navigator.storage.persist());
    } catch {
      persistenceWarning = true;
    }
  }

  async function request(path: string, init?: RequestInit) {
    try {
      const response = await fetch(path, { ...init, cache: 'no-store', signal: AbortSignal.timeout(8000) });
      const body = response.status === 200 ? await response.text() : '';
      connection = 'online';
      return { status: response.status, body };
    } catch {
      connection = 'offline';
      return null;
    }
  }

  function readJSON(body: string): unknown {
    try {
      return JSON.parse(body);
    } catch {
      throw new Error('서버 응답을 읽지 못했습니다.');
    }
  }

  async function runSession(operation: () => Promise<void>) {
    if (busy) return;
    busy = true;
    error = '';
    try {
      await operation();
    } catch (cause) {
      error = cause instanceof Error && cause.name === 'Error'
        ? cause.message : '기기 저장소를 읽거나 쓸 수 없습니다.';
    } finally {
      busy = false;
      opening = false;
      if (retryQueued) {
        retryQueued = false;
        retryConnection();
      }
    }
  }

  function retryConnection() {
    if (busy) retryQueued = true;
    else void runSession(restoreSession);
  }

  async function restoreSession() {
    const session = await readSession();
    if (session.logoutPending) {
      await finishLogout();
      return;
    }
    await loadAccount(session.user);
  }

  async function showUser(next: User | null) {
    if (next?.userId === user?.userId) {
      user = next;
      return;
    }

    user = null;
    records = [];
    saved = false;
    if (!next) return;

    const [catalog, ownRecords] = await Promise.all([
      readEquipment(), readRecords(next.userId)
    ]);
    equipment = catalog.equipment;
    syncedAt = catalog.syncedAt;
    records = ownRecords;
    user = next;
  }

  async function loadAccount(cachedUser: User | null) {
    const response = await request('/api/me');
    if (response === null) {
      await showUser(cachedUser);
      return;
    }
    if (response.status === 401) {
      await showUser(null);
      await cacheUser(null);
      return;
    }
    if (response.status !== 200) throw new Error('로그인 상태를 확인하지 못했습니다.');

    const account = parseUser(readJSON(response.body));
    await cacheUser(account);
    await showUser(account);

    const catalog = await request('/api/equipment');
    if (catalog === null) return;
    if (catalog.status === 401) {
      await showUser(null);
      await cacheUser(null);
      return;
    }
    if (catalog.status !== 200) throw new Error('설비 목록을 갱신하지 못했습니다.');

    const items = parseEquipmentList(readJSON(catalog.body));
    const now = Date.now();
    await replaceEquipment(items, now);
    equipment = items;
    syncedAt = now;
  }

  function login(event: SubmitEvent) {
    event.preventDefault();
    if (!(event.currentTarget instanceof HTMLFormElement)) return;
    const fields = new FormData(event.currentTarget);
    void runSession(async () => {
      if ((await readSession()).logoutPending && !await finishLogout()) return;

      const response = await request('/api/session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ userId: fields.get('userId'), password: fields.get('password') })
      });
      if (response === null) return;
      if (response.status === 401) throw new Error('아이디 또는 비밀번호가 올바르지 않습니다.');
      if (response.status !== 204) throw new Error('로그인하지 못했습니다.');
      await loadAccount(null);
    });
  }

  function logout() {
    void runSession(async () => {
      await beginLogout();
      await showUser(null);
      await finishLogout();
    });
  }

  async function finishLogout() {
    const response = await request('/api/session/logout', { method: 'POST' });
    if (response === null) return false;
    if (response.status !== 204) throw new Error('서버에서 로그아웃을 완료하지 못했습니다.');
    await completeLogout();
    return true;
  }

  async function save(input: { equipmentId: EquipmentId; result: Result; memo: string }) {
    if (!user) return false;
    const authorId = user.userId;
    saved = false;
    error = '';
    try {
      const record = createRecord({ ...input, id: crypto.randomUUID(), authorId, inspectedAt: Date.now() });
      await saveRecord(record);
      if (user?.userId === authorId) {
        records = [record, ...records];
        saved = true;
      }
      return true;
    } catch {
      if (user?.userId === authorId) error = '기기에 점검을 저장하지 못했습니다. 다시 시도하세요.';
      return false;
    }
  }
</script>

<svelte:head>
  <title>현장 점검</title>
  <meta name="theme-color" content="#12372e" />
</svelte:head>

<header class="masthead">
  <div class="container">
    <div class="masthead-row">
      <h1>현장 점검</h1>
      {#if user}
        <div class="account">
          <span>{user.name}</span>
          <button type="button" class="secondary" onclick={logout} aria-disabled={busy}>로그아웃</button>
        </div>
      {/if}
    </div>
    {#if user}
      <dl class="metrics">
        <div><dt>연결 상태</dt><dd>{connectionLabels[connection]}</dd></div>
        <div><dt>저장된 설비</dt><dd>{equipment.length}대</dd></div>
        <div><dt>마지막 설비 갱신</dt><dd>{syncedAt === null ? '갱신 이력 없음' : dateTime.format(syncedAt)}</dd></div>
        <div><dt>전송 대기</dt><dd>{pendingCount}건</dd></div>
      </dl>
    {/if}
  </div>
</header>

<main class="container">
  <div class="visually-hidden" aria-live="polite">{validationMessage || (saved ? '기기에 저장됨' : '')}</div>
  {#if persistenceWarning}
    <p class="notice">저장소 보존이 허용되지 않았습니다. 브라우저의 ‘홈 화면에 추가’를 이용해 앱을 열어 주세요.</p>
  {/if}
  {#if error}
    <p class="notice error-notice" role="alert">오류: {error}</p>
  {/if}

  {#if opening}
    <p>로그인 상태를 확인하는 중입니다.</p>
  {:else if !user}
    <section class="panel login-panel" aria-labelledby="login-heading">
      <h2 id="login-heading">로그인</h2>
      {#if connection === 'offline'}
        <p>로그인하려면 인터넷에 연결하세요.</p>
      {/if}
      <form method="post" onsubmit={login}>
        <div>
          <label for="user-id">아이디</label>
          <input id="user-id" name="userId" type="text" autocomplete="username"
            autocapitalize="none" autocorrect="off" spellcheck="false" required
            data-error="user-error" aria-invalid="false" />
          <p class="field-error" id="user-error">오류: 아이디를 입력하세요.</p>
        </div>
        <div>
          <label for="password">비밀번호</label>
          <input id="password" name="password" type="password" autocomplete="current-password" required
            data-error="password-error" aria-invalid="false" />
          <p class="field-error" id="password-error">오류: 비밀번호를 입력하세요.</p>
        </div>
        <button type="submit" aria-disabled={busy}>{busy ? '확인 중…' : '로그인'}</button>
      </form>
    </section>
  {:else}
    {#if saved}
      <p class="notice saved">기기에 저장됨</p>
    {/if}
    <div class="workspace">
      <InspectionForm {equipment} {save} />
      <section class="panel" aria-labelledby="records-heading">
        <h2 id="records-heading">내 기록</h2>
        {#if records.length}
          <ul class="records">
            {#each records as record (record.id)}
              {@const { equipmentId, inspectedAt, memo, result } = record.payload}
              <li>
                <div class="record-heading">
                  <strong>{equipmentId} {equipmentNames.get(equipmentId) ?? ''}</strong>
                  <span class="record-state">{syncLabels[record.sync.kind]}</span>
                </div>
                <p>{result === 'normal' ? '정상' : '이상'} · <time datetime={new Date(inspectedAt).toISOString()}>{dateTime.format(inspectedAt)}</time></p>
                {#if memo}<p class="record-memo">{memo}</p>{/if}
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted">저장된 점검 기록이 없습니다.</p>
        {/if}
      </section>
    </div>
  {/if}
</main>
