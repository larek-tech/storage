package kafka

// Message[T] — типизированная обёртка над RawMessage.
// Если декодирование не удалось, Value содержит zero-value, а UnmarshalErr — ошибку.
// Handler сам решает, пропустить такое сообщение, отправить в DLQ или прервать обработку.
type Message[T any] struct {
	Value        T
	Raw          RawMessage
	UnmarshalErr error
}
