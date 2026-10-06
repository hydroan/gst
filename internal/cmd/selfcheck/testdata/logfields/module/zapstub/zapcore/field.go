package zapcore

type Field struct{ Key string }

type ObjectEncoder interface {
	AddString(key, value string)
	AddInt64(key string, value int64)
}

type ObjectMarshaler interface{ MarshalLogObject(ObjectEncoder) error }

type ArrayEncoder interface{ AppendString(string) }

type ArrayMarshaler interface{ MarshalLogArray(ArrayEncoder) error }
