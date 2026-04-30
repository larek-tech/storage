# s3

Минимальная обёртка над S3 / S3-compatible хранилищами для пакета `larek-tech/storage`.

## Минимальные требования

- Сохранение объектов (PutObject)
- Получение объектов (GetObject)
- Получение списка по префиксу (ListObjects)
- Генерация ссылки для прямой загрузки (PresignPutObject)

## Конфигурация

```go
cfg := s3.Cfg{
    Region:          "eu-central-1",
    Bucket:          "my-bucket",
    Endpoint:        "https://s3.amazonaws.com", // опционально
    AccessKeyID:     "key",                      // опционально
    SecretAccessKey: "secret",                   // опционально
    UsePathStyle:    false,                       // true для MinIO/LocalStack
}
```

Обязательные поля: `Region`, `Bucket`. Если задан один из `AccessKeyID`/`SecretAccessKey`, должен быть задан и второй. При отсутствии кредов используется стандартная цепочка AWS (env/shared config/role).

## Использование

```go
client, err := s3.New(ctx, cfg, s3.WithTelemetry(true))
if err != nil {
    panic(err)
}

err = client.PutObject(ctx, "path/to/file.txt", bytes.NewReader(data), "text/plain")

obj, err := client.GetObject(ctx, "path/to/file.txt")
if err != nil {
    // handle
}
defer obj.Body.Close()

items, err := client.ListObjects(ctx, "path/")

url, err := client.PresignPutObject(ctx, "path/to/upload.bin", 10*time.Minute)
```

## Замечания

- `GetObject` возвращает `*s3.GetObjectOutput`. Не забывайте закрывать `Body`.
- При использовании S3-compatible endpoint задавайте `Endpoint` и при необходимости `UsePathStyle`.
