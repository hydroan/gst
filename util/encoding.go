package util

import (
	"reflect"
	"strconv"

	"github.com/cockroachdb/errors"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func Marshal(value any) ([]byte, error) {
	switch val := value.(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	case int:
		return []byte(strconv.FormatInt(int64(val), 10)), nil
	case int8:
		return []byte(strconv.FormatInt(int64(val), 10)), nil
	case int16:
		return []byte(strconv.FormatInt(int64(val), 10)), nil
	case int32:
		return []byte(strconv.FormatInt(int64(val), 10)), nil
	case int64:
		return []byte(strconv.FormatInt(val, 10)), nil
	case uint:
		return []byte(strconv.FormatUint(uint64(val), 10)), nil
	case uint8:
		return []byte(strconv.FormatUint(uint64(val), 10)), nil
	case uint16:
		return []byte(strconv.FormatUint(uint64(val), 10)), nil
	case uint32:
		return []byte(strconv.FormatUint(uint64(val), 10)), nil
	case uint64:
		return []byte(strconv.FormatUint(val, 10)), nil
	case bool:
		return []byte(strconv.FormatBool(val)), nil
	case float32:
		return []byte(strconv.FormatFloat(float64(val), 'f', -1, 32)), nil
	case float64:
		return []byte(strconv.FormatFloat(val, 'f', -1, 64)), nil
	}
	return json.Marshal(value)
}

func Unmarshal(data []byte, value any) error {
	if value == nil {
		return errors.New("Unmarshal: value is nil")
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("Unmarshal: value must be a non-nil pointer")
	}

	switch v := value.(type) {
	case *string:
		*v = string(data)
		return nil
	case *[]byte:
		*v = data
		return nil
	case *int:
		n, err := strconv.ParseInt(string(data), 10, 0)
		*v = int(n)
		return err
	case *int8:
		n, err := strconv.ParseInt(string(data), 10, 8)
		*v = int8(n)
		return err
	case *int16:
		n, err := strconv.ParseInt(string(data), 10, 16)
		*v = int16(n)
		return err
	case *int32:
		n, err := strconv.ParseInt(string(data), 10, 32)
		*v = int32(n)
		return err
	case *int64:
		n, err := strconv.ParseInt(string(data), 10, 64)
		*v = n
		return err
	case *uint:
		n, err := strconv.ParseUint(string(data), 10, 0)
		*v = uint(n)
		return err
	case *uint8:
		n, err := strconv.ParseUint(string(data), 10, 8)
		*v = uint8(n)
		return err
	case *uint16:
		n, err := strconv.ParseUint(string(data), 10, 16)
		*v = uint16(n)
		return err
	case *uint32:
		n, err := strconv.ParseUint(string(data), 10, 32)
		*v = uint32(n)
		return err
	case *uint64:
		n, err := strconv.ParseUint(string(data), 10, 64)
		*v = n
		return err
	case *bool:
		b, err := strconv.ParseBool(string(data))
		*v = b
		return err
	case *float32:
		f, err := strconv.ParseFloat(string(data), 32)
		*v = float32(f)
		return err
	case *float64:
		f, err := strconv.ParseFloat(string(data), 64)
		*v = f
		return err
	default:
		return json.Unmarshal(data, value)
	}
}
