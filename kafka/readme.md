# kafka

Generic-обёртка над Kafka для пакетов `larek-tech/storage`. Заменяет паттерн «сгенерировать `ProduceXxx`/`ConsumeXxx` на каждый топик» на типизированные через дженерики `Producer[T]` и `Consumer[T]`.

- **Транспорт**: [`segmentio/kafka-go`](https://github.com/segmentio/kafka-go), pure-Go (CGO не нужен).
- **OpenTelemetry**: опциональная трассировка produce/consume.
- **Retry / DLQ**: pluggable интерфейсы; есть in-memory реализации **только для тестов**.

## Установка

```sh
go get github.com/larek-tech/storage/kafka
```

## Архитектура

```
┌──────────────────────────────┐
│  Producer[T] / Consumer[T]   │  типизированный API + Codec[T]
├──────────────────────────────┤
│  Transport (RawMessage)      │  работает с байтами
├──────────────────────────────┤
│  KafkaGoTransport            │  segmentio/kafka-go реализация
└──────────────────────────────┘
```

Дженерики живут только в верхнем слое. `Transport` — простой интерфейс над байтами, его легко мокать в тестах или подменять на другую реализацию.

## Конфигурация

```go
cfg := kafka.Cfg{
    Brokers:  []string{"b1:9092", "b2:9092"},
    GroupID:  "my-service",
    ClientID: "my-service-1",
    UseTLS:   true,
    SASL:     "scram-sha-256",
    User:     "kafka",
    Password: "secret",
}
```

Поддерживается DSN-форма:

```go
cfg, err := kafka.NewCfgFromDSN(
    "kafka://kafka:secret@b1:9092,b2:9092/?group_id=my-service&sasl=scram-sha-256&tls=true",
)
```

## Producer

```go
type Booking struct {
    ID    string  `json:"id"`
    Total float64 `json:"total"`
}

tr, err := kafka.NewKafkaGoTransport(ctx, cfg)
if err != nil { /* ... */ }
defer tr.Close()

bookings := kafka.NewProducer[Booking](tr, "str-booking-storage.booking",
    kafka.WithKeyExtractor[Booking](func(b Booking) []byte { return []byte(b.ID) }),
    kafka.WithProducerTelemetry[Booking](true),
)

_ = bookings.Produce(ctx, []Booking{{ID: "1", Total: 99.95}})
```

Опции:

| Опция | Назначение |
|---|---|
| `WithProducerCodec[T](Codec[T])` | заменить кодек по умолчанию (JSON) |
| `WithKeyExtractor[T](func(T) []byte)` | извлечь partition key из значения |
| `WithProducerTelemetry[T](bool)` | включить OTel-спаны |
| `WithProducerTracer[T](trace.Tracer)` | свой Tracer вместо `otel.Tracer(...)` |

`Produce` сериализует каждый элемент через `Codec[T]` и отправляет батчем через транспорт.

## Consumer

```go
type StatusChanged struct {
    BookingID string `json:"booking_id"`
    Status    string `json:"status"`
}

statuses := kafka.NewConsumer[StatusChanged](tr, "str-booking-storage.booking.status.changed")

err := statuses.Consume(ctx, func(ctx context.Context, msgs []kafka.Message[StatusChanged]) (int, error) {
    for _, m := range msgs {
        if m.UnmarshalErr != nil {
            // битое сообщение — решает handler: skip, DLQ, log, etc.
            continue
        }
        log.Printf("booking %s -> %s (offset=%d)", m.Value.BookingID, m.Value.Status, m.Raw.Offset)
    }
    return len(msgs), nil
})
```

`Message[T]`:

```go
type Message[T any] struct {
    Value        T          // десериализованное значение (zero-value, если UnmarshalErr != nil)
    Raw          RawMessage // ключ, заголовки, partition, offset, timestamp
    UnmarshalErr error      // не nil → handler сам решает, что делать
}
```

Handler возвращает `(processed int, err error)`. Транспорт коммитит оффсеты только если `err == nil` и `processed >= 1`.

Опции:

| Опция | Назначение |
|---|---|
| `WithConsumerCodec[T](Codec[T])` | заменить кодек |
| `WithRetryPolicy[T](RetryPolicy)` | retry на ошибках handler |
| `WithDLQ[T](DLQ)` | DLQ после исчерпания ретраев |
| `WithConsumerTelemetry[T](bool)` | OTel-спаны |
| `WithConsumerTracer[T](trace.Tracer)` | свой Tracer |

## Codec[T]

Любая (де)сериализация с типом `T`:

```go
type Codec[T any] interface {
    Marshal(v T) ([]byte, error)
    Unmarshal(b []byte, v *T) error
}
```

По умолчанию — `JSONCodec[T]` на `encoding/json` (поддерживает `json:"..."` теги). Свой кодек подключается через `WithProducerCodec` / `WithConsumerCodec` (например, для protobuf).

## Retry и DLQ

Интерфейсы:

```go
type RetryPolicy interface {
    Next(attempt int, err error) (delay time.Duration, retry bool)
}

type DLQ interface {
    Send(msg RawMessage, reason error) error
}
```

Поведение `Consumer[T].Consume`:

1. Handler возвращает ошибку → опросить `RetryPolicy.Next(attempt, err)`.
2. `retry == true` → `time.Sleep(delay)` (с уважением к `ctx`) и повтор.
3. Ретраи исчерпаны и подключен `DLQ` → каждое сообщение батча отправляется в DLQ, ошибка не пробрасывается.
4. Без `RetryPolicy` — ошибка handler пробрасывается транспорту как есть.

### In-memory реализации (НЕ для продакшена)

```go
import pkgkafka "github.com/larek-tech/storage/kafka"

retry := pkgkafka.InMemoryRetryPolicy{
    MaxAttempts: 3,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    5 * time.Second,
}
dlq := &pkgkafka.InMemoryDLQ{}

consumer := pkgkafka.NewConsumer[StatusChanged](tr, topic,
    pkgkafka.WithRetryPolicy[StatusChanged](retry),
    pkgkafka.WithDLQ[StatusChanged](dlq),
)
```

`InMemoryRetryPolicy` хранит счётчики per-process и теряет прогресс на рестарте; `InMemoryDLQ` хранит сообщения в slice — данные не персистентны и не реплицируются. В проде используйте отдельный Kafka-топик, базу или внешний store.

## Подмена транспорта

`Transport` — простой интерфейс:

```go
type Transport interface {
    Produce(ctx context.Context, topic string, msgs []RawMessage) error
    Consume(ctx context.Context, topic string, handler func([]RawMessage) (int, error)) error
    Close() error
}
```

В тестах легко подменить на in-memory мок и проверять `Producer[T]` / `Consumer[T]` без брокера.

## Ограничения текущей реализации

- `KafkaGoTransport.Consume` отдаёт по одному сообщению в handler (`batch_size = 1`). Коммит после успешного handler. Для батчинга на стороне транспорта потребуется расширение API.
- Нет встроенной поддержки транзакций / exactly-once.
- Нет встроенных хуков под Schema Registry / Avro — `Codec[T]` — generic-интерфейс над байтами, без специальных гарантий совместимости с Confluent SR.

## CGO

Все рантайм-зависимости pure-Go. Сборка с `CGO_ENABLED=0` поддерживается — проверено в CI.

## Тестирование

Юнит-тесты — рядом с пакетом:

```sh
go test ./...
```

Интеграционные тесты с реальным брокером вынесены в отдельный go-модуль `kafka/integration/`, чтобы пользователи не получали `testcontainers-go` транзитивно. Запуск:

```sh
cd kafka/integration && go test ./...
```

В CI эти тесты гоняются автоматически workflow'ом `.github/workflows/kafka-integration.yaml` при изменениях в `kafka/**` против `confluentinc/confluent-local:8.0.0` (Apache Kafka 4.0, KRaft).
