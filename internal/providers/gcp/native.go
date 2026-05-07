package gcp

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// nativeFrom returns v as a json.RawMessage suitable for Resource.Native when
// dumpNative is true, otherwise returns nil.
//
// Proto messages must go through protojson — encoding/json produces empty
// objects for proto-generated structs because their fields are unexported.
// Non-proto values (e.g., sqladmin or storage structs) fall back to standard
// json.Marshal.
func nativeFrom(dumpNative bool, v any) any {
	if !dumpNative || v == nil {
		return nil
	}
	if msg, ok := v.(proto.Message); ok {
		b, err := protojson.Marshal(msg)
		if err != nil {
			return nil
		}
		return json.RawMessage(b)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}
