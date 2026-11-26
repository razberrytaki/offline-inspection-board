# 설계 노트

README에서 다루지 않은 동작 규칙과 API, 데이터 구조를 정리한 문서다.

## 동작 규칙

- 직원은 온라인일 때 설비 목록을 기기에 받아 둔다. 목록은 IndexedDB 트랜잭션 하나로 통째로 교체하므로, 받다가 끊기면 이전 목록이 남는다.
- 점검 결과는 '정상' 또는 '이상'이고, '이상'이면 메모를 꼭 적어야 한다. 저장한 점검은 기기에서도 고칠 수 없고 정정으로만 바꾼다.
- 정정은 원본 작성자만 할 수 있다. 정정은 원본의 설비와 점검 시각을 그대로 쓰고 결과나 메모만 바꾼다. 원본은 `revision` 0이고 정정은 1, 2, 3 순서로 올라간다.
- 공용 기기에서는 기록마다 작성자를 저장하고, 지금 로그인한 사람의 기록만 보낸다. 다른 직원이 로그인해도 앞 사람의 기록이 그 직원 이름으로 접수되지 않는다.
- 보드는 서버 접수 시각 순이다. 늦게 동기화된 점검도 접수된 순간 맨 위에 나타나고, 정정이 접수되면 그 줄이 맨 위로 올라간다. 단순 재전송은 순서를 바꾸지 않는다.

## API

| 경로 | 설명 |
|---|---|
| `POST /api/session` | `{userId, password}`로 로그인하고 세션 쿠키를 받는다 |
| `POST /api/session/logout` | 세션을 지운다 |
| `GET /api/me` | `{userId, name, role}` |
| `GET /api/equipment` | 폐기되지 않은 설비 목록 |
| `PUT /api/inspections/{id}` | 점검 또는 정정을 접수한다 |
| `GET /api/board` | 원본마다 최신 revision 한 줄씩, 최근 접수 순 (감독자) |
| `GET /api/board/{rootId}` | 원본과 모든 정정의 이력 (감독자) |
| `GET /api/board/events` | 새 접수를 SSE로 보낸다 (감독자) |

PUT 요청 본문은 다음과 같다. 작성자는 본문에 넣지 않고 서버가 세션에서 정한다. 점검 시각은 UTC 밀리초다.

```json
{
  "rootId": "원본 ID (원본이면 경로의 ID와 같음)",
  "revision": 0,
  "equipmentId": "P-12",
  "result": "abnormal",
  "memo": "가동 중 누수 발견",
  "inspectedAt": 1764130000000
}
```

| 상황 | 응답 | 직원 화면 |
|---|---|---|
| 처음 받은 ID | 201, `{id, receivedAt}` | 동기화 완료 |
| 같은 ID, 같은 내용 | 200, 처음 접수 시각 | 동기화 완료 |
| 같은 ID, 다른 내용 또는 다른 작성자 | 409 `content_mismatch` | 확인 필요 |
| 같은 revision이 이미 있음 | 409 `revision_taken` | 확인 필요 |
| 앞선 revision이 없음 | 409 `missing_previous` | 확인 필요 |
| 남의 원본을 정정 | 403 | 확인 필요 |
| 형식 오류, 모르는 설비 | 422 | 확인 필요 |
| 세션 만료 | 401 | 다시 로그인 필요 (기록은 전송 대기 유지) |
| 네트워크 오류, 시간 초과, 5xx, 429 | 없음 | 전송 대기, 재시도 |

서버는 메모를 NFC로 정규화하고 앞뒤 공백을 지운 뒤, 내용의 SHA-256을 저장해 같은 내용인지 비교한다. 기기는 재시도할 때 처음 저장한 본문을 그대로 보낸다.

## 데이터 모델

```sql
create table inspections (
  id            uuid primary key,          -- 기기가 만듦
  root_id       uuid not null references inspections(id),
  revision      int  not null,             -- 원본은 0
  equipment_id  text not null references equipment(id),
  result        text not null check (result in ('normal', 'abnormal')),
  memo          text not null default '',
  inspected_at  timestamptz not null,      -- 기기 시계
  author_id     text not null references users(id),
  content_hash  bytea not null,
  received_at   timestamptz not null default now(),  -- 서버 시계
  unique (root_id, revision)
);
```

PUT은 트랜잭션 하나에서 처리한다. 먼저 ID로 기존 기록을 찾아 있으면 해시를 비교한다. 없으면 `INSERT ... ON CONFLICT DO NOTHING`으로 넣는다. 이 한 문장이 ID 중복과 `(root_id, revision)` 중복을 모두 흡수하므로, 행이 들어가지 않았으면 다시 조회해 200이나 409로 답한다. 새 행이 들어갔을 때만 커밋 후 보드 알림을 보낸다.

## 기기의 기록 상태

| 상태 | 화면 | 뜻 |
|---|---|---|
| `pending` | 전송 대기 | 서버 접수를 아직 확인하지 못함 |
| `synced` | 동기화 완료 | 201 또는 200을 받음 |
| `needs_attention` | 확인 필요 | 409, 403, 422를 받음 |

- 대기 기록은 점검 시각, 원본 ID, revision 순으로 하나씩 보낸다. 네트워크 오류가 나면 거기서 멈추고, 1초부터 두 배씩 최대 60초 간격으로 다시 시도한다. 그래서 정정이 원본보다 먼저 나가지 않는다.
- 결과는 저장된 상태가 아직 `pending`일 때만 반영한다. 탭 두 개가 동시에 보내도 결과가 같다.
- '확인 필요' 기록은 서버 기록과 나란히 보여주고, 새 점검으로 다시 제출하거나 기기에서 지울 수 있다.
- 연결 상태는 `navigator.onLine`이 아니라 마지막 요청이 성공했는지로 판단한다.

## 장애 패널

`FAULT_INJECTION=1`로 서버를 띄우면 직원 화면에 나타난다. 인증이 없는 개발용 기능이라 공개 배포에서는 꺼야 한다.

- **서버 연결 끊기.** 서버가 `/api/dev` 외의 모든 API 연결을 응답 없이 끊는다. 열려 있던 보드 SSE도 끊긴다.
- **저장 직후 응답 끊기.** 다음 PUT 하나를 저장과 알림까지 끝낸 뒤 응답 없이 끊는다.
- **같은 점검 5번 동시 전송.** 최근 동기화된 기록의 PUT을 다섯 번 동시에 보낸다.

이 모드에서는 서버가 HTTP 연결 재사용을 끈다. 재사용하던 연결이 끊기면 Chromium이 요청을 알아서 다시 보내므로, 그대로 두면 앱의 재시도를 시험할 수 없다.

## 개발 환경

Go, Node.js 24 이상, pnpm 12, PostgreSQL 17이 필요하다.

```sh
docker compose up -d --wait postgres
pnpm -C web install
DATABASE_URL='postgres://postgres:dev@127.0.0.1:5433/oib?sslmode=disable' go run ./cmd/server
pnpm -C web dev   # 다른 터미널, /api는 Go 서버로 넘김
```

- 서비스 워커까지 확인하려면 `pnpm -C web build` 후 Go 서버 주소(`http://localhost:8080`)를 연다. Go 서버는 `STATIC_DIR`(기본값 `web/build`)의 파일을 서빙한다.
- `scripts/build.sh`는 화면 파일을 넣은 단일 바이너리 `bin/oib`를 만든다(`-tags embed`).
- 테스트는 실행할 때마다 임시 DB를 만들고 지운다. 접속 주소는 `TEST_DATABASE_URL`로 바꿀 수 있다. Playwright 서버 포트는 `E2E_PORT`(기본값 18091)다.

## 한계

- 앱이 닫혀 있으면 전송하지 않는다. Safari에는 Background Sync가 없다.
- iOS Safari는 홈 화면에 추가하지 않은 사이트의 저장소를 지울 수 있다. 앱은 `navigator.storage.persist()`를 요청하고, 거절되면 경고를 띄운다. 실제 iPhone에서는 아직 확인하지 않았다.
- 알림은 서버 프로세스 안에서만 전달한다. 서버를 여러 대 띄우려면 PostgreSQL `LISTEN/NOTIFY` 같은 수단이 필요하다.
