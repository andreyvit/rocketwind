package rocketwind

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
)

// ConfigEntry is one entry in an ordered configuration object.
type ConfigEntry struct {
	Key   string
	Value any
}

// ConfigObject preserves configuration order where it affects CSS, such as
// overlapping custom breakpoints. Values may be strings, numbers, booleans,
// nested objects, slices or Go theme functions. JSON decoding preserves order.
// A map[string]any is also accepted wherever a nested object is expected; its
// keys are visited in lexical order.
type ConfigObject []ConfigEntry

func (o ConfigObject) Get(key string) any {
	for _, entry := range o {
		if entry.Key == key {
			return entry.Value
		}
	}
	return nil
}

func (o *ConfigObject) set(key string, value any) {
	for i := range *o {
		if (*o)[i].Key == key {
			(*o)[i].Value = value
			return
		}
	}
	*o = append(*o, ConfigEntry{key, value})
}

func (o ConfigObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	buf.WriteByte('{')
	for i, entry := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		_ = encoder.Encode(entry.Key)
		buf.Truncate(buf.Len() - 1)
		buf.WriteByte(':')
		if err := encoder.Encode(entry.Value); err != nil {
			return nil, err
		}
		buf.Truncate(buf.Len() - 1)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (o *ConfigObject) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("configuration must be an object")
	}
	var result ConfigObject
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		value, err := decodeConfigValue(raw)
		if err != nil {
			return err
		}
		result.set(key.(string), value)
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected data after configuration object")
	}
	*o = result
	return nil
}

func decodeConfigValue(raw []byte) (any, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty configuration value")
	}
	if raw[0] == '{' {
		var object ConfigObject
		err := json.Unmarshal(raw, &object)
		return object, err
	}
	if raw[0] == '[' {
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		values := make([]any, len(entries))
		for i, entry := range entries {
			value, err := decodeConfigValue(entry)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	}
	var value any
	err := json.Unmarshal(raw, &value)
	return value, err
}

func configObject(value any) ConfigObject {
	switch value := value.(type) {
	case ConfigObject:
		if value == nil {
			return ConfigObject{}
		}
		return value
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		result := make(ConfigObject, 0, len(keys))
		for _, key := range keys {
			result = append(result, ConfigEntry{key, value[key]})
		}
		return result
	case map[string]string:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		result := make(ConfigObject, 0, len(keys))
		for _, key := range keys {
			result = append(result, ConfigEntry{key, value[key]})
		}
		return result
	}
	return nil
}

func configGet(value any, path []string) any {
	for i := 0; i < len(path); i++ {
		key := path[i]
		next := configObject(value).Get(key)
		if array, ok := value.([]any); ok {
			if index, err := strconv.Atoi(key); err == nil && index >= 0 && index < len(array) {
				next = array[index]
			}
		}
		if next == nil {
			if i+1 == len(path) {
				return nil
			}
			path = slices.Clone(path)
			path[i+1] = key + "-" + path[i+1]
			continue
		}
		value = next
	}
	return value
}

func cloneConfigValue(value any) any {
	if object := configObject(value); object != nil {
		result := make(ConfigObject, len(object))
		for i, entry := range object {
			result[i] = ConfigEntry{entry.Key, cloneConfigValue(entry.Value)}
		}
		return result
	}
	if array, ok := value.([]any); ok {
		result := make([]any, len(array))
		for i, entry := range array {
			result[i] = cloneConfigValue(entry)
		}
		return result
	}
	if value != nil {
		v := reflect.ValueOf(value)
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			result := make([]any, v.Len())
			for i := range result {
				result[i] = cloneConfigValue(v.Index(i).Interface())
			}
			return result
		}
	}
	return value
}

func mergeConfigValue(base, extra any) any {
	left, right := configObject(base), configObject(extra)
	if array, ok := base.([]any); ok && len(array) > 0 && configObject(array[0]) != nil {
		result := cloneConfigValue(array).([]any)
		if values, ok := extra.([]any); ok {
			return append(result, cloneConfigValue(values).([]any)...)
		}
		return append(result, cloneConfigValue(extra))
	}
	if values, ok := extra.([]any); ok && len(values) > 0 && configObject(values[0]) != nil && left != nil {
		return append([]any{cloneConfigValue(left)}, cloneConfigValue(values).([]any)...)
	}
	if right != nil {
		result := configObject(cloneConfigValue(left))
		for _, entry := range right {
			result.set(entry.Key, mergeConfigValue(result.Get(entry.Key), entry.Value))
		}
		return result
	}
	return cloneConfigValue(extra)
}
