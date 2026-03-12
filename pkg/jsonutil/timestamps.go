package jsonutil

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

var (
	timeType          = reflect.TypeOf(time.Time{})
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

func FormatUTCTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func FormatUTCTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}

	s := FormatUTCTime(*t)
	return &s
}

func MarshalNormalized(v any) ([]byte, error) {
	normalized, err := normalizeJSONValue(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}

	return json.Marshal(normalized)
}

func normalizeJSONValue(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}

	if v.Type() == timeType {
		return FormatUTCTime(v.Interface().(time.Time)), nil
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return nil, nil
		}
		return normalizeJSONValue(v.Elem())
	case reflect.Pointer:
		if v.IsNil() {
			return nil, nil
		}
		if v.Elem().Type() == timeType {
			return FormatUTCTime(v.Elem().Interface().(time.Time)), nil
		}
		if typeImplementsJSONMarshaler(v.Type()) || typeImplementsTextMarshaler(v.Type()) {
			return v.Interface(), nil
		}
		return normalizeJSONValue(v.Elem())
	}

	if typeImplementsJSONMarshaler(v.Type()) || typeImplementsTextMarshaler(v.Type()) {
		return v.Interface(), nil
	}

	switch v.Kind() {
	case reflect.Struct:
		obj := make(map[string]any)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" && !field.Anonymous {
				continue
			}

			if shouldFlattenAnonymousField(field, field.Tag.Get("json")) {
				normalized, err := normalizeJSONValue(v.Field(i))
				if err != nil {
					return nil, err
				}
				embedded, ok := normalized.(map[string]any)
				if !ok {
					return normalized, nil
				}
				for key, value := range embedded {
					obj[key] = value
				}
				continue
			}

			name, omitEmpty, skip := parseJSONTag(field)
			if skip {
				continue
			}

			fieldValue := v.Field(i)
			if omitEmpty && isEmptyJSONValue(fieldValue) {
				continue
			}

			normalized, err := normalizeJSONValue(fieldValue)
			if err != nil {
				return nil, err
			}
			obj[name] = normalized
		}
		return obj, nil
	case reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
		if v.Type().Key().Kind() != reflect.String {
			return v.Interface(), nil
		}

		obj := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			normalized, err := normalizeJSONValue(iter.Value())
			if err != nil {
				return nil, err
			}
			obj[iter.Key().String()] = normalized
		}
		return obj, nil
	case reflect.Slice:
		if v.IsNil() {
			return nil, nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return v.Interface(), nil
		}
		fallthrough
	case reflect.Array:
		items := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			normalized, err := normalizeJSONValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			items[i] = normalized
		}
		return items, nil
	default:
		return v.Interface(), nil
	}
}

func parseJSONTag(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}

	name = field.Name
	if tag == "" {
		return name, false, false
	}

	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func isEmptyJSONValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	case reflect.Struct:
		return v.IsZero()
	}
	return false
}

func typeImplementsJSONMarshaler(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType)
}

func typeImplementsTextMarshaler(t reflect.Type) bool {
	return t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)
}

func shouldFlattenAnonymousField(field reflect.StructField, rawTag string) bool {
	if !field.Anonymous {
		return false
	}

	if rawTag != "" && !strings.HasPrefix(rawTag, ",") {
		return false
	}

	fieldType := field.Type
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	if fieldType.Kind() != reflect.Struct {
		return false
	}

	if fieldType == timeType || typeImplementsJSONMarshaler(field.Type) || typeImplementsTextMarshaler(field.Type) {
		return false
	}

	return true
}
