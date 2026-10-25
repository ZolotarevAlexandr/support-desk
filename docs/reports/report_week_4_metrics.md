# Сбор и хранение метрик

В Minikube-кластере развёрнута VictoriaMetrics. Приложение Support Desk экспортирует метрики Go и процесса,
а также пять бизнес-метрик: `support_tickets_opened_total`, `support_ticket_actions_total`,
`support_comments_added_total`, `support_logins_total` и `support_ticket_resolution_duration_seconds`.
PostgreSQL экспортирует показатели через готовый postgres-exporter, запущенный вторым контейнером в Pod базы данных.
Приложение и PostgreSQL работают в namespace `support-desk`, VictoriaMetrics - в `monitoring`.

## Компоненты и конфигурация

| Компонент           | Версия / настройка                                        |
|---------------------|-----------------------------------------------------------|
| Helm-чарт           | `victoria-metrics-single`, 0.49.0, release `vm-single`    |
| VictoriaMetrics     | `victoriametrics/victoria-metrics:v1.154.0`, одна реплика |
| Приложение          | `support-desk:metrics-v1`, Go 1.27.1, Gin                 |
| PostgreSQL          | 18.0                                                      |
| PostgreSQL exporter | `quay.io/prometheuscommunity/postgres-exporter:v0.20.1`   |
| Хранение метрик     | PVC 2 GiB, StorageClass `standard`, retention 7 дней      |
| Сбор                | Pull, каждые 15 секунд, timeout 5 секунд                  |

VictoriaMetrics установлена официальным Helm-чартом. Values и манифесты находятся в `k8s/monitoring/`.
Single-node сервер самостоятельно обнаруживает таргеты, опрашивает их и сохраняет временные ряды.
Service `victoria-metrics` предоставляет внутренний адрес на порту 8428. Подробные команды развёртывания и
ожидания готовности после запуска Minikube приведены в `k8s/monitoring/README.md`.

## Экспорт метрик приложения

Использована библиотека `github.com/prometheus/client_golang` 1.25.0. Готовые `collectors.NewGoCollector()` и
`collectors.NewProcessCollector()` экспортируют показатели goroutines, памяти, сборки мусора и процесса:

```promql
go_goroutines{job="support-desk"}
go_memstats_heap_alloc_bytes{job="support-desk"}
process_resident_memory_bytes{job="support-desk"}
process_cpu_seconds_total{job="support-desk"}
```

Метрики доступны без авторизации на отдельном порту 9090 по пути `/metrics`. `promhttp.HandlerFor` с
`EnableOpenMetrics: true` поддерживает OpenMetrics при соответствующем заголовке `Accept`.
Service приложения публикует только порт 8080; scraper обращается к внутренним адресам Pod на порту 9090.
Реализация находится в `backend/internal/metrics/`.

## Бизнес-метрики

| Метрика                                      | Тип / метки            | Что измеряет                                                     |
|----------------------------------------------|------------------------|------------------------------------------------------------------|
| `support_tickets_opened_total`               | Counter, `priority`    | Число успешно сохранённых новых заявок по начальному приоритету. |
| `support_ticket_actions_total`               | Counter, `action`      | Число успешно завершённых действий над заявками.                 |
| `support_comments_added_total`               | Counter, `author_role` | Число явно добавленных комментариев пользователя или оператора.  |
| `support_logins_total`                       | Counter, `role`        | Число успешных входов с сохранённой сессией.                     |
| `support_ticket_resolution_duration_seconds` | Histogram, `priority`  | Возраст заявки в секундах при успешном разрешении оператором.    |

Метрики обновляются после успешной операции хранилища, для транзакционных изменений - после commit.
Отказы в доступе, ошибки валидации, конфликты и откаты не увеличивают счётчики успешных операций.
Автоматические workflow-сообщения исключены из счётчика комментариев, регистрация исключена из счётчика явных входов.

Метки ограничены приоритетами `low`, `medium`, `high`, `urgent`, действиями `claim`, `reassign`, `resolve`, `confirm`,
`request_changes`, `reopen` и ролями `user`, `operator`.
Идентификаторы пользователей и заявок, email и тексты сообщений в метки не попадают.
Гистограмма измеряет возраст от первоначального создания заявки; каждый повторный успешный `resolve` добавляет
новое наблюдение. Счётчики сбрасываются при перезапуске процесса; для прироста и скорости событий используются
`increase()` и `rate()` соответственно.

## Метрики PostgreSQL

Готовый postgres-exporter запущен отдельным контейнером в том же Pod, что и PostgreSQL, и подключается к `127.0.0.1:5432`.
Имя базы и пользователя берёт из ConfigMap, пароль - из Secret через `secretKeyRef`.
Метрики экспортируются на порту 9187 по пути `/metrics`; конфигурация находится в `k8s/postgres.yaml`.

```promql
pg_up{job="postgres"}
pg_exporter_last_scrape_error{job="postgres"}
pg_stat_database_numbackends{job="postgres",datname="support"}
```

## Service discovery

В `spec.template.metadata.annotations` Deployment приложения и PostgreSQL добавлены:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "9090"  # Для Pod PostgreSQL: "9187"
prometheus.io/path: "/metrics"
```

Scraper использует `kubernetes_sd_configs` с `role: pod` в namespace `support-desk`.
Job `support-desk` выбирает приложение, job `postgres` - экспортёр базы. Relabeling читает аннотации,
выбирает объявленный порт, совпадающий с аннотированным, и добавляет метки `namespace`, `pod`, `container`.
Это позволяет собирать метрики каждой реплики без статических IP и повторных скрейпов из-за нескольких портов Pod.
ServiceAccount VictoriaMetrics получает права `get`, `list`, `watch` на Pod через Role и RoleBinding.
Сбор выполняет встроенный scraper single-node сервера, его статус доступен на `/targets` порта 8428.
