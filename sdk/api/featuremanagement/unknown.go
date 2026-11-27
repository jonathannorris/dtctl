package featuremanagement

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// unknownFields holds the members of a JSON object that its Go type does not
// declare. Feature Management writes replace what they cover and clear what a
// request leaves out, so a read-modify-write built against an older spec
// would silently drop every field the server added since (design decision
// 3). The types that dtctl reads and sends back keep those members here and
// write them out again unchanged.
type unknownFields map[string]json.RawMessage

// decodeKeepingUnknown decodes data into v, a pointer to a struct type with
// no custom UnmarshalJSON (callers pass an alias of their own type), and
// returns the object members v's type does not declare.
func decodeKeepingUnknown(data []byte, v any) (unknownFields, error) {
	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	known := jsonFieldNames(reflect.TypeOf(v).Elem())
	var unknown unknownFields
	for k, raw := range members {
		if known[k] {
			continue
		}
		if unknown == nil {
			unknown = unknownFields{}
		}
		unknown[k] = raw
	}
	return unknown, nil
}

// encodeKeepingUnknown encodes v, an alias value of a struct type with no
// custom MarshalJSON, and adds the unknown members back. Without unknown
// members the encoding is exactly v's own, field order included.
func encodeKeepingUnknown(v any, unknown unknownFields) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil || len(unknown) == 0 {
		return data, err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	for k, raw := range unknown {
		if _, declared := members[k]; !declared {
			members[k] = raw
		}
	}
	return json.Marshal(members)
}

var jsonFieldNamesCache sync.Map // reflect.Type -> map[string]bool

// jsonFieldNames returns the JSON member names a struct type declares.
func jsonFieldNames(t reflect.Type) map[string]bool {
	if cached, ok := jsonFieldNamesCache.Load(t); ok {
		return cached.(map[string]bool)
	}
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case "":
			name = f.Name
		}
		names[name] = true
	}
	jsonFieldNamesCache.Store(t, names)
	return names
}
