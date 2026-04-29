package kafka

import "encoding/json"

// Codec кодирует и декодирует одно сообщение типа T.
type Codec[T any] interface {
	Marshal(v T) ([]byte, error)
	Unmarshal(b []byte, v *T) error
}

// JSONCodec — кодек по умолчанию на encoding/json. Поддерживает любые
// `json:"..."` теги.
type JSONCodec[T any] struct{}

func (JSONCodec[T]) Marshal(v T) ([]byte, error)    { return json.Marshal(v) }
func (JSONCodec[T]) Unmarshal(b []byte, v *T) error { return json.Unmarshal(b, v) }
