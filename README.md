# 오프라인 현장 점검 보드

[![CI](https://github.com/razberrytaki/offline-inspection-board/actions/workflows/ci.yml/badge.svg)](https://github.com/razberrytaki/offline-inspection-board/actions/workflows/ci.yml)

통신이 안 되는 현장에서 설비 점검을 기록하고, 연결이 돌아오면 감독자 보드에 바로 뜨는 웹 앱이다. Go와 SvelteKit으로 만들었다.

## 문제의식

서버는 저장했는데 응답이 중간에 사라지면, 기기는 저장이 됐는지 알 수 없다. 그러니 다시 보내야 하는데, 다시 보낸다고 점검이 두 건이 되면 안 된다.

## 어떻게 풀었나

- **ID는 기기가 만든다.** 점검을 저장하는 순간 기기가 UUID를 만들고, 내용과 함께 IndexedDB에 먼저 넣는다. 전송은 `PUT /api/inspections/{id}`라서 몇 번을 보내도 같은 점검을 가리킨다.
- **서버는 처음 받은 것만 저장한다.** 같은 ID에 같은 내용이 다시 오면 새로 저장하지 않고 처음 접수한 시각을 돌려준다. 내용이 다르면 409로 거절한다. 같은 요청이 동시에 와도 DB 고유 제약이 한 건만 남긴다.
- **정정은 덮어쓰지 않고 쌓는다.** 정정할 때마다 `revision`이 하나씩 올라가고, 보드는 원본마다 가장 최신 revision만 보여준다. 원본은 그대로 남아서 상세 화면에서 이력을 볼 수 있다.
- **보드는 SSE로 갱신한다.** 다시 연결될 때는 들어오는 이벤트를 모아 두고, 목록을 새로 받은 다음 모아 둔 이벤트를 적용한다. 끊긴 사이에 들어온 점검도 빠지지 않는다.

## 실행

```sh
docker compose up --build
```

http://localhost:8080 을 연다. 데모 계정은 직원 `worker-1` / `demo-worker-1`, 감독자 `supervisor` / `demo-supervisor`다.

## 데모

직원은 일반 창에서, 감독자는 시크릿 창에서 로그인한다. 직원 화면의 **장애 패널**에서 서버 연결을 끊거나 응답을 일부러 잃어버리게 할 수 있다.

1. **서버 연결 끊기**를 누르고 `P-12 냉각수 펌프`를 '이상'으로 기록한다. 탭을 닫았다 다시 열어도 **전송 대기**로 남아 있다.
2. **저장 직후 응답 끊기**를 켜고 **서버 연결 복구**를 누른다. 첫 응답은 사라지지만 앱이 다시 보내서 **동기화 완료**가 되고, 보드에는 한 줄만 생긴다. 패널에 보이는 서버 행 수도 1만 늘어난다.
3. 그 점검의 메모를 정정하면 보드의 기존 줄이 맨 위로 올라간다. **같은 점검 5번 동시 전송**을 눌러도 행 수와 접수 시각은 바뀌지 않는다.

## 테스트

```sh
docker compose up -d --wait postgres
go test -race ./...
pnpm -C web install
pnpm -C web test
pnpm -C web test:e2e
```

Go 테스트는 실제 PostgreSQL에 같은 PUT 50개를 동시에 보내 한 건만 저장되는지 확인한다. Playwright 테스트는 직원과 감독자 브라우저를 따로 띄워 위 데모 흐름을 그대로 돌린다.

## 범위

사진 첨부, 설비별 체크리스트, 앱이 닫혀 있는 동안의 백그라운드 전송은 만들지 않았다. 브라우저 테스트는 Chromium에서만 돌렸다.

API, 기기의 기록 상태, DB 스키마, 개발 환경은 [docs/design.md](docs/design.md)에 정리했다.
