# Алерты Support Desk

Команды выполняются из корня репозитория. Нужны Docker, kubectl и работающий [SLO-стенд](../slo/README.md).
Правила и причины выбора порогов — в [отчёте](../../docs/reports/report_week_6_alerts.md).

## Настройка и запуск

Создайте бота через @BotFather и отправьте ему `/start`. Впишите числовой `chat_id`
в [alertmanager.yml](alertmanager.yml). Добавьте токен в существующий `.env`
в корне репозитория, который уже исключён из git:

```dotenv
TELEGRAM_BOT_TOKEN=ваш_токен
```

```sh
./k8s/alerting/deploy.sh
```

Скрипт читает `.env`, создаёт/обновляет Secret `alertmanager-telegram` из
`TELEGRAM_BOT_TOKEN`, загружает публичные YAML в ConfigMap, применяет манифесты и перезапускает
компоненты для перечитывания файлов. Другой контекст: `KUBE_CONTEXT=name ./k8s/alerting/deploy.sh`.
Alertmanager сохраняет silences и состояние уведомлений на PVC 1 GiB.
vmalert пишет историю и состояние алертов в существующий VictoriaMetrics и читает их при рестарте.

В отдельных терминалах:

```sh
kubectl --context=minikube -n monitoring port-forward service/vmalert 18880:8880
kubectl --context=minikube -n monitoring port-forward service/alertmanager 19093:9093
```

Правила: http://localhost:18880/vmalert/groups (Expand All).
Полученные алерты: http://localhost:19093. После рестарта Pod переподключите port-forward.

## Демонстрация

Дождитесь пяти минут здорового трафика пробера. Включите ошибки бизнес-операций:

```sh
kubectl --context=minikube -n support-desk set env deployment/support-app LAB_FAULT_RATE=1 LAB_FAULT_DELAY=0s
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
```

После выполнения условия нужны 2 минуты `for` и до 30 секунд группировки уведомлений.
Снимите скриншоты правил и Telegram-уведомления, затем верните обычные настройки стенда:

```sh
kubectl --context=minikube -n support-desk set env deployment/support-app LAB_FAULT_RATE=0 LAB_FAULT_DELAY=0s
kubectl --context=minikube -n support-desk rollout status deployment/support-app --timeout=180s
```

Для runtime-демо загрузите вариант с явно указанными демонстрационными порогами 0:

```sh
kubectl --context=minikube -n monitoring create configmap support-alert-rules \
  --from-file=business.yaml=k8s/alerting/business.yaml \
  --from-file=runtime.yaml=k8s/alerting/runtime-demo.yaml --dry-run=client -o yaml |
  kubectl --context=minikube -n monitoring apply -f -
kubectl --context=minikube -n monitoring rollout restart deployment/vmalert
kubectl --context=minikube -n monitoring rollout status deployment/vmalert --timeout=180s
```

`profile=demo` и текст уведомлений поясняют: это проверка доставки на настоящих метриках,
а не утечка или нехватка памяти. Дождитесь FIRING в Telegram, затем верните обычные правила:

```sh
kubectl --context=minikube -n monitoring create configmap support-alert-rules \
  --from-file=business.yaml=k8s/alerting/business.yaml \
  --from-file=runtime.yaml=k8s/alerting/runtime-normal.yaml --dry-run=client -o yaml |
  kubectl --context=minikube -n monitoring apply -f -
kubectl --context=minikube -n monitoring rollout restart deployment/vmalert
kubectl --context=minikube -n monitoring rollout status deployment/vmalert --timeout=180s
```

Alertmanager оставьте запущенным. Когда истечёт срок действия последних demo-алертов,
он отправит RESOLVED; на это и группировку уведомлений отведите до 5 минут.
`deploy.sh` для этого шага не подходит: он перезапускает и Alertmanager.
PVC сохраняет silences и историю отправленных уведомлений, но не сами активные алерты.
При рестарте Alertmanager demo-алерты теряются, и уведомления об их завершении могут не прийти.

После выключения сбоев бизнес-алерты очистятся за 5 минут плюс время вычисления.
Для завершения сдачи нужны скриншоты созданных правил, Telegram-срабатывания и краткое
объяснение условий. Дополнительные скриншоты восстановления полезны, но не обязательны.

## Проверка

```sh
docker run --rm -v "$PWD/k8s/alerting:/rules:ro" victoriametrics/vmalert:v1.154.0 \
  -rule=/rules/business.yaml -rule=/rules/runtime-normal.yaml -dryRun
docker run --rm -v "$PWD/k8s/alerting:/rules:ro" -w /rules \
  --entrypoint=/bin/promtool prom/prometheus:v3.2.1 test rules rules.test.yaml
kubectl --context=minikube -n monitoring exec deployment/alertmanager -- \
  /bin/amtool check-config /etc/alertmanager/alertmanager.yml
```

Четыре сценария проверяют firing/recovery, точные пороги, недостаточный трафик
и агрегацию реплик со сбросом counter. При проблемах смотрите `kubectl logs` у
`deployment/vmalert` и `deployment/alertmanager` в namespace `monitoring`.
