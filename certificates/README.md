# Сертификаты Минцифры

Публичные CA-сертификаты скопированы из `/usr/local/share/ca-certificates/`
локальной машины:

- `Russian_Trusted_Root_CA.crt` — корневой CA;
- `Russian_Trusted_Sub_CA.crt` — промежуточный CA 2022 года;
- `Russian_Trusted_Sub_CA_2024.crt` — промежуточный CA 2024 года.

Dockerfile устанавливает их через `update-ca-certificates` в системный bundle
Alpine `/etc/ssl/certs/ca-certificates.crt`. Он используется Go и системными
HTTPS-клиентами. Переменные `SSL_CERT_FILE`/`SSL_CERT_DIR` для образа не нужны.
Приватных ключей здесь нет. После обновления сертификатов пересоберите образ.
