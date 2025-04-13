# Пакет работы с PostgreSQL 

Этот пакет предоставляет клиент базы данных PostgreSQL с интеграцией OpenTelemetry и поддержкой управления транзакциями для Go-приложений.

## Установка

```
go get github.com/larek-tech/storage/postgres
```

## Возможности

- Простая настройка подключения через конфигурацию
- Управление транзакциями через [go-transaction-manager](https://github.com/avito-tech/go-transaction-manager)
- Встроенная трассировка операций с базой данных через OpenTelemetry
- Вспомогательные методы для распространенных операций с базой данных

## Конфигурация

Используйте структуру `Cfg` для настройки подключения к базе данных:

```go
cfg := postgres.Cfg{
    User:     "postgres",
    Password: "password",
    Host:     "localhost",
    Port:     5432,
    DB:       "mydb",
}
```

Конфигурация также может быть загружена из переменных окружения:
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_HOST`
- `POSTGRES_PORT`
- `POSTGRES_DB`

## Использование

### Базовое подключение

```go
import (
    "context"
    "github.com/larek-tech/storage/postgres"
)

func main() {
    cfg := postgres.Cfg{
        User:     "postgres",
        Password: "password",
        Host:     "localhost",
        Port:     5432,
        DB:       "mydb",
    }
    
    ctx := context.Background()
    db, trManager, err := postgres.New(ctx, cfg)
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // Используйте db для операций с базой данных
}
```

### Включение OpenTelemetry

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

// Включить телеметрию с трейсером по умолчанию
db, trManager, err := postgres.New(ctx, cfg, 
    postgres.WithTelemetry(true))

// Или с пользовательским трейсером
tracer := otel.Tracer("my-app")
db, trManager, err := postgres.New(ctx, cfg, 
    postgres.WithTelemetry(true),
    postgres.WithTracer(tracer))
```

### Основные операции с базой данных

```go
// Выполнить запрос
rows, err := db.Query(ctx, "SELECT id, name FROM users WHERE age > $1", 18)
if err != nil {
    // Обработка ошибки
}
defer rows.Close()

// Выполнить запрос и сканировать результаты в структуру
type User struct {
    ID   int    `db:"id"`
    Name string `db:"name"`
}

var user User
err = db.QueryStruct(ctx, &user, "SELECT id, name FROM users WHERE id = $1", 1)

// Выполнить запрос и сканировать результаты в срез структур
var users []User
err = db.QueryStructs(ctx, &users, "SELECT id, name FROM users WHERE age > $1", 18)

// Выполнить операцию
result, err := db.Exec(ctx, "UPDATE users SET name = $1 WHERE id = $2", "New Name", 1)
```

### Работа с транзакциями

```go
import (
    "github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

// Начать транзакцию
err := trManager.Do(ctx, func(ctx context.Context) error {
    // Все операции с базой данных внутри этой функции будут использовать одну транзакцию
    
    // Добавить нового пользователя
    _, err := db.Exec(ctx, "INSERT INTO users(name, age) VALUES($1, $2)", "Alice", 30)
    if err != nil {
        return err // Транзакция будет отменена
    }
    
    // Обновить другую запись
    _, err = db.Exec(ctx, "UPDATE users SET age = $1 WHERE name = $2", 31, "Bob")
    if err != nil {
        return err // Транзакция будет отменена
    }
    
    return nil // Транзакция будет зафиксирована
})
```

## Трассировка

Когда телеметрия включена, все операции с базой данных автоматически создают спаны со следующими атрибутами:
- Текст SQL-запроса (без конфиденциальных значений параметров)
- Количество аргументов запроса
- Количество затронутых строк (для операций Exec)
- Детали ошибок (когда операции завершаются с ошибкой)

Интеграция трассировки работает без проблем с экосистемой OpenTelemetry.