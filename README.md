# Telegram Timer

Go 기반 Telegram Bot Webhook 알림 서버. "HH:mm 메시지" 형식으로 오늘 알림을 등록하면 **30분 전 / 10분 전 / 5분 전**과 **지정 시간**에 텔레그램으로 알림을 보냅니다.

## 요구사항

- Docker & Docker Compose (권장), 또는 Go 1.21+
- Telegram Bot Token ([@BotFather](https://t.me/BotFather)에서 발급)

## 빠른 시작 (Docker)

```bash
git clone https://github.com/your-username/telegram-timer.git
cd telegram-timer
cp .env.example .env
# .env 파일을 열어 BOT_TOKEN을 실제 토큰으로 수정
docker compose up -d
```

서비스는 `http://localhost:8080`에서 동작합니다. Telegram 봇이 이 서버로 메시지를 보내려면 **Webhook**을 등록해야 합니다 (아래 "Webhook 설정" 참고).

## 로컬 실행 (Go)

```bash
export BOT_TOKEN=your_bot_token
export DB_PATH=./data/reminders.db   # 선택, 기본값 동일
go run main.go
```

서버는 기본적으로 `:8080`에서 기동합니다. `ADDR` 환경변수로 변경 가능합니다.

## 환경변수

| 변수 | 필수 | 설명 |
|------|------|------|
| `BOT_TOKEN` | 예 | Telegram Bot API 토큰 |
| `DB_PATH` | 아니오 | SQLite DB 파일 경로 (기본: `./data/reminders.db`, Docker 기본: `/data/reminders.db`) |
| `ADDR` | 아니오 | HTTP 리스닝 주소 (기본: `:8080`) |

## Webhook 설정

Telegram이 메시지를 이 서버로 보내려면 **HTTPS** Webhook URL이 필요합니다.

1. 서버를 공개 HTTPS URL에서 실행 (예: VPS + nginx 리버스 프록시, 또는 ngrok).
2. Webhook 등록:

```bash
curl -X POST "https://api.telegram.org/bot<BOT_TOKEN>/setWebhook" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://your-domain.com/telegram/webhook"}'
```

로컬 테스트 시 [ngrok](https://ngrok.com/) 등으로 HTTPS 터널을 열고, 해당 URL을 Webhook으로 설정하면 됩니다.

## 사용법

### 일회성 알림

- **알림 등록**: `15:30 회의 일정` — 오늘 15:30에 "회의 일정" 알림 등록 (HH:mm 형식, 이미 지난 시간은 불가). `MM/dd HH:mm 메시지` 형식으로 특정 날짜 알림도 등록 가능.
- **목록**: `/list` — 오늘 기준 미발송 **알림** 목록만 조회 (루틴 제외).
- **삭제**: `/delete 1` — `/list`에서 1번 항목 삭제.

각 알림은 **30분 전**, **10분 전**, **5분 전**, **지정 시간**에 총 4회 발송됩니다.

### 루틴 (반복 알림)

일회성 알림과 달리 **루틴만** 요일·반복 규칙을 씁니다.

- **매일**: `/r HH:mm 메시지` — 예: `/r 09:00 물 마시기`
- **매주 (요일 지정)**: `/r [요일 부분] HH:mm 메시지`
  - **요일 부분**은 **문장 맨 앞**에만 올 수 있고, **첫 번째 `HH:mm`**이 시간·메시지 경계입니다 (그 뒤는 전부 메시지).
  - **단일 요일**: `/r 월 08:00 주간 회의`
  - **여러 요일 (쉼표)**: `/r 월, 수, 금 12:00 약`
  - **범위 (월→일 순만)**: `/r 월-금 18:00 퇴근`, `/r 토-일 10:00 브런치` — `금-월`처럼 **역방향·건너뛰는 범위는 불가**.
  - **단축**: `평일`(월–금), `주말`(토·일) — 예: `/r 평일 09:00 출근`, `/r 주말 11:00 늦잠`
- **루틴 목록**: `/r-list` — 등록된 루틴만 조회.
- **루틴 삭제**: `/r-delete 1` — `/r-list`의 1번 루틴 삭제.
- **도움말**: `/r` 만 보내면 위 형식 요약이 옵니다.

## Docker 상세

- 이미지 빌드: `docker compose build`
- 백그라운드 실행: `docker compose up -d`
- 로그: `docker compose logs -f app`
- 중지: `docker compose down` (볼륨은 유지되므로 DB는 보존됨)

`BOT_TOKEN`은 `.env`에 두면 `docker compose`가 자동으로 주입합니다. `.env`는 git에 올리지 마세요.

홈서버에서 기존 docker-compose에 서비스를 합치려면, 이 디렉터리를 빌드 컨텍스트로 두고 `telegram-timer` 서비스를 추가한 뒤, nginx에서 `location /telegram/webhook { proxy_pass http://telegram-timer:8080; }` 로 프록시하면 됩니다.

## 프로젝트 구조

```
.
├── main.go           # HTTP 서버, Webhook 라우팅, Scheduler 기동
├── config/           # BOT_TOKEN, DB_PATH
├── db/               # SQLite 연결 및 마이그레이션
├── handler/          # Webhook 핸들러
├── service/          # Reminder CRUD, Scheduler
├── telegram/         # sendMessage 클라이언트
├── Dockerfile
├── docker-compose.yml
├── .env.example      # 복사 후 .env로 BOT_TOKEN 설정
└── README.md
```
