# clientapp — Mini App клиента

Запись на консультацию и свои записи, для клиента. Отдельное приложение, а не
раздел админки: клиент входит подписанным запуском Telegram, сотрудник —
паролем, и общий бандл им не нужен. Статический экспорт, авторизация только
`tma`.

Бэкенд: `/client/booking-options`, `/client/requests`, `/client/me` — скоупы
`client` и `guest`, см. `backend/api/openapi.yaml`.

## Локально

```sh
# backend/.env: CF_TELEGRAM_DEV_USER_ID=<любой id>, и :3001 в CF_APP_CORS_ALLOWED_ORIGINS
make dev            # бэкенд :8080 + админка :3000
make dev-clientapp  # это приложение :3001
```

`CF_TELEGRAM_DEV_USER_ID` — дырка в аутентификации: подпись запуска не
проверяется. Без неё приложение нельзя открыть в обычном браузере вообще,
потому что подписать запуск может только Telegram и только для https-origin.
Сервер отказывается стартовать, если переменная задана при неlocalhost
`CF_APP_ADMIN_URL`.

## Прод

Telegram не откроет Mini App по http. На проде (132.243.194.137) порт 80 держит
Caddy соседнего проекта `tracker-bot`, а 443 — посторонний relay `amnezia-telemt`,
поэтому TLS терминирует соседский Caddy, а наш контейнер отдаёт статику по
`127.0.0.1:3002`. Весь роутинг приложения остаётся здесь — соседу нужна одна
проксирующая строка.

**1. В нашем `backend/.env`:**

```
TELEGRAM_MINIAPP_URL=https://abalis-132-243-194-137.sslip.io:8443/
```

Имя `abalis-<ip через дефисы>.sslip.io` резолвится на этот адрес без всякой
регистрации. Свой домен лучше (Let's Encrypt лимитирует выдачу на общий
`sslip.io`), но для старта хватает. Пустая переменная просто убирает кнопку
приложения из бота, ничего не ломая.

**2. Поднять контейнер:**

```sh
docker compose -f devops/compose.yaml --env-file backend/.env --profile clientapp up --build -d
```

Без `--profile clientapp` он не стартует.

**3. В проекте `tracker-bot` (владелец 80 и 8443) — добавить сайт в Caddyfile:**

```caddy
abalis-132-243-194-137.sslip.io {
	# Тот же обход, что у самого tracker-bot: TLS-ALPN пошёл бы на 443, а он
	# занят посторонним сервисом. HTTP-01 на 80 работает, и сертификат не
	# привязан к порту, на котором его потом отдают.
	tls {
		issuer acme {
			disable_tlsalpn_challenge
		}
	}
	reverse_proxy host.docker.internal:3002
}
```

и в его compose, сервису с Caddy:

```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

Без `extra_hosts` контейнер соседа не увидит `127.0.0.1:3002` хоста — для него
это его собственный localhost.

Порт HTTPS у соседа глобально 8443, поэтому адрес приложения — с портом. Он
должен совпадать с `TELEGRAM_MINIAPP_URL` дословно.

**4. BotFather** (не обязательно, кнопка в сообщении работает и так):

- `/mybots` → бот → Bot Settings → Menu Button → URL — кнопка рядом с полем
  ввода, доступна всегда
- `/newapp` — прямая ссылка `t.me/<бот>/<имя>`, удобно рассылать в смс вместо
  `/intakelink`
