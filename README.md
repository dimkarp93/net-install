# net-install

Загрузки из сети с повторными попытками и структурным логом.

Раньше это была shell-библиотека `lib/net.sh` в репозитории конфигов, которую
сорсил каждый скрипт установки. Теперь это отдельная программа: её можно поставить
в `PATH` и звать откуда угодно.

## Установка

```sh
github_install.sh dimkarp93/net-install
```

Из рабочей копии:

```sh
local_install.sh net-install
```

## Команды

```
net-install fetch    [флаги] URL DEST        скачать URL в DEST ("-" - в stdout)
net-install download [флаги] URL DEST        скачать во временный файл и установить в DEST
net-install script   [флаги] URL [АРГ...]    скачать скрипт и выполнить; АРГ уходят скрипту
net-install clone    [флаги] URL DEST        git clone --depth 1, пропускает существующий DEST
net-install apt      [флаги] ПАКЕТ...        sudo apt-get install -y с кэшем .deb
net-install log      KEY=VALUE...            напечатать строку лога, без обращения к сети
net-install env                              действующие NET_* и откуда они взялись
```

`download` отличается от `fetch` тем, что пишет во временный файл и только потом
переносит его в `DEST` - в том числе через `sudo install -D -m`, если каталог
назначения не доступен на запись. `fetch` пишет в `DEST` напрямую.

Разбор флагов останавливается на первом позиционном аргументе, поэтому всё, что
идёт после URL, достаётся скрипту нетронутым:

```sh
net-install script --shell bash https://just.systems/install.sh --to ~/.local/bin
```

## Переменные окружения

| Переменная | Флаг | По умолчанию | Что делает |
|---|---|---|---|
| `NET_RETRIES` | `--retries` | 5 | число попыток |
| `NET_DELAY` | `--delay` | 5 | пауза между попытками, сек |
| `NET_CONNECT_TIMEOUT` | `--connect-timeout` | 20 | таймаут соединения, сек |
| `NET_SPEED_LIMIT` | `--speed-limit` | 1024 | порог скорости, байт/сек |
| `NET_SPEED_TIME` | `--speed-time` | 30 | сколько держаться ниже порога до обрыва, сек |
| `NET_SHELL` | `--shell` | `sh` | интерпретатор для `script` |
| `NET_MODE` | `--mode` | `0644` | права на `DEST` для `download` |
| `NET_RETRY_ALL` | `--retry-all-errors` | выкл | повторять и 4xx тоже |
| `NET_CACHE_DIR` | `--cache-dir` | `/mnt/hdd/auto-distrib` | каталог кэша |
| `NET_NO_CACHE` | `--no-cache` | выкл | работать без кэша |
| `NET_FORCE_UPDATE` | `--force-update` | выкл | скачать заново и обновить кэш |

Флаг сильнее переменной, переменная сильнее умолчания. `net-install env` показывает,
что именно действует сейчас:

```
$ NET_RETRIES=9 net-install env --delay 2
NET_RETRIES=9 (env)
NET_DELAY=2 (flag)
NET_CONNECT_TIMEOUT=20 (default)
...
```

Загрузка обрывается и повторяется, если скорость держится ниже `NET_SPEED_LIMIT`
дольше `NET_SPEED_TIME` - зависшее соединение не блокирует установку. `clone`
передаёт те же два значения в `git` как `http.lowSpeedLimit` и `http.lowSpeedTime`.

## Кэш

`fetch`, `download`, `script`, `clone` и `apt` складывают скачанное в
`NET_CACHE_DIR` и при следующем вызове берут оттуда, не выходя в сеть:

```
/mnt/hdd/auto-distrib/
  files/<host>/<path>            fetch, download, script
  files/<host>/<path>.sha256     сверяется перед каждым использованием
  git/<host>/<owner>/<repo>/     shallow-зеркало для clone
  apt/*.deb                      пакеты, скачанные apt-get
```

URL с query-строкой получает к имени суффикс `@<первые 12 символов sha256(query)>`.

- Файл берётся из кэша, если он есть и его sha256 совпадает с записанной. Иначе он
  скачивается заново.
- `clone` клонирует из локального зеркала и затем возвращает `origin` на исходный URL.
- `apt` перед установкой копирует недостающие `.deb` из кэша в
  `/var/cache/apt/archives`, после установки забирает новые обратно. Какую версию
  ставить, решает сам apt по своим индексам, поэтому `apt update` по-прежнему нужен.
- `--force-update` скачивает всё заново и обновляет кэш: файлы перекачиваются,
  зеркало делает `git fetch`, существующий `DEST` у `clone` клонируется повторно, у `apt`
  пропускается подкладывание `.deb`. Если обновить файл или зеркало не удалось, а
  старая копия есть, используется она (`event=cache-stale`).
- Если каталога нет, net-install создаёт его, но только когда существует родитель. Если
  каталог создать нельзя или он недоступен на запись, команда падает с кодом 1 и ничего
  не скачивает. Работать без кэша можно только явно, через `--no-cache`.

Дополнительные события лога: `cache-hit`, `cache-store`, `cache-stale`.

## Что повторяется, а что нет

Повторяются таймауты, обрывы, ошибки DNS и соединения, стоп по скорости, HTTP 5xx,
429 и 408, а также любой ненулевой выход `git clone`.

Остальные 4xx и локальные ошибки падают сразу, на первой же попытке: повторять 404
бессмысленно. Старая shell-версия повторяла и их - если нужно прежнее поведение,
есть `--retry-all-errors` (`NET_RETRY_ALL=1`).

## Лог

`stderr`, строки `key=value`:

```
[net] ts=2026-09-20T01:54:11Z event=start what=fetch https://github.com/... attempt=1/5
[net] ts=2026-09-20T01:55:31Z event=retry what=fetch https://github.com/... attempt=1/5 rc=7 seconds=80 sleep=5s
[net] ts=2026-09-20T01:55:58Z event=ok what=fetch https://github.com/... attempt=2 seconds=22
[net] ts=2026-09-20T01:55:58Z event=size what=fetch url=https://github.com/... bytes=61521920
```

События: `start`, `ok`, `retry`, `fail`, `size`, `install`, `exec`, `done`, `skip`,
`cache-hit`, `cache-store`, `cache-stale`.

## Коды возврата

`0` успех, `1` локальная ошибка, `2` ошибка употребления, `6` DNS, `7` соединение,
`18` оборванное тело ответа, `22` HTTP >= 400, `28` таймаут или стоп по скорости,
`35` TLS, `47` слишком много редиректов.

`script` и `clone` вместо этого пробрасывают код дочернего процесса как есть
(`128+сигнал`, если процесс убит) - на это опираются вызывающие скрипты с `set -e`.

## Ограничения

Бинарь собирается с `CGO_ENABLED=0`, поэтому корневые сертификаты читаются чисто
Go-загрузчиком: `/etc/ssl/certs`, `SSL_CERT_FILE`, `SSL_CERT_DIR`. Сертификат,
добавленный только в NSS через `certutil`, виден `curl`, но не виден здесь.

Ветка с эскалацией прав зовёт `install -D -m`, то есть GNU coreutils. На macOS это
ограничение проявится только для каталогов, недоступных пользователю на запись.

## Сборка

```sh
just build
just check
```

## Релиз

```sh
just release patch
```

`versions.txt` поднимается, коммитится и пушится; тег и релиз создаёт workflow.

Программа следует конвенциям
<https://github.com/dimkarp93/install/blob/master/CONVENTIONS.md>.
