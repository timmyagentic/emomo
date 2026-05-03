// Package persistence wires GORM to the generated protobuf message schema.
//
// The protojson serializer registered here lets domain GORM models declare
// JSON columns whose Go type is a generated *pb.X message directly, without
// any hand-written newtype wrapper. The wire format is protojson with
// UseEnumNumbers=true + UseProtoNames=true, which is byte-compatible with the
// pre-refactor encoding/json output that the previous hand-written
// domain.ImageInfo / domain.AnnotationLabels Value/Scan pair produced (legacy
// payloads of the form {"width":W,"height":H,"format":N} and similar are
// still accepted by this serializer's Unmarshal path).
package persistence

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/schema"
)

// ProtojsonSerializer marshals/unmarshals generated protobuf messages to/from
// the JSON form used by the relational columns. Register name: "protojson".
type ProtojsonSerializer struct{}

var (
	marshalProtojson   = protojson.MarshalOptions{UseEnumNumbers: true, UseProtoNames: true}
	unmarshalProtojson = protojson.UnmarshalOptions{DiscardUnknown: true}
)

const emptyJSONObject = "{}"

func init() {
	schema.RegisterSerializer("protojson", ProtojsonSerializer{})
}

// Scan decodes the database value into the field on dst. It tolerates nil and
// empty payloads by leaving the field as a freshly-allocated zero message,
// preserving the historical "non-null column with empty struct" semantics.
func (ProtojsonSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	bytes, err := scanJSONBytes(dbValue)
	if err != nil {
		return err
	}

	msgPtr, err := newMessagePointer(field.FieldType)
	if err != nil {
		return err
	}

	if len(bytes) > 0 {
		if err := unmarshalProtojson.Unmarshal(bytes, msgPtr.Interface().(proto.Message)); err != nil {
			return fmt.Errorf("protojson scan %s: %w", field.Name, err)
		}
	}

	field.ReflectValueOf(ctx, dst).Set(msgPtr)
	return nil
}

// Value encodes the field value into a JSON string suitable for a TEXT
// column. nil pointer fields serialize to "{}" so columns declared NOT NULL
// in the migrations always observe a syntactically-valid JSON payload.
func (ProtojsonSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue any) (any, error) {
	if fieldValue == nil {
		return emptyJSONObject, nil
	}

	msg, ok := fieldValue.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("protojson value %s: type %T is not a proto.Message", field.Name, fieldValue)
	}
	if reflect.ValueOf(msg).IsNil() {
		return emptyJSONObject, nil
	}

	b, err := marshalProtojson.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("protojson value %s: %w", field.Name, err)
	}
	if len(b) == 0 {
		return emptyJSONObject, nil
	}
	return string(b), nil
}

// MarshalProtoColumn produces the JSON string that would be stored when GORM
// writes a *pb.X message to a column tagged `serializer:protojson`. Use this
// for `Updates(map[string]any{...})` paths where GORM does not run the
// schema-field serializer.
func MarshalProtoColumn(msg proto.Message) (string, error) {
	if msg == nil {
		return emptyJSONObject, nil
	}
	if reflect.ValueOf(msg).IsNil() {
		return emptyJSONObject, nil
	}
	b, err := marshalProtojson.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("protojson marshal: %w", err)
	}
	if len(b) == 0 {
		return emptyJSONObject, nil
	}
	return string(b), nil
}

// newMessagePointer allocates a zero value for a proto-message-pointer field
// type. It returns the pointer as a reflect.Value so callers can both write
// to the field and pass the underlying message to protojson.Unmarshal.
func newMessagePointer(fieldType reflect.Type) (reflect.Value, error) {
	if fieldType.Kind() != reflect.Ptr {
		return reflect.Value{}, fmt.Errorf("protojson serializer: field type %s must be a pointer to a proto message", fieldType.String())
	}
	msgPtr := reflect.New(fieldType.Elem())
	if _, ok := msgPtr.Interface().(proto.Message); !ok {
		return reflect.Value{}, fmt.Errorf("protojson serializer: type %s is not a proto.Message", fieldType.String())
	}
	return msgPtr, nil
}

func scanJSONBytes(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, errors.New("protojson serializer: unsupported db value type")
	}
}
