import { parseBoardDetail } from '$lib/board';
import type { PageLoad } from './$types';

type DetailState =
  | { kind: 'login' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; detail: ReturnType<typeof parseBoardDetail> };

export const load: PageLoad = async ({ fetch, params }): Promise<DetailState> => {
  try {
    const response = await fetch('/api/board/' + encodeURIComponent(params.rootId), {
      cache: 'no-store', signal: AbortSignal.timeout(8000)
    });
    if (response.status === 401 || response.status === 403) return { kind: 'login' };
    if (response.status === 404) return { kind: 'error', message: '해당 점검 기록을 찾을 수 없습니다.' };
    if (!response.ok) throw new Error();
    return { kind: 'ready', detail: parseBoardDetail(await response.json(), params.rootId) };
  } catch {
    return { kind: 'error', message: '점검 상세를 불러오지 못했습니다.' };
  }
};
