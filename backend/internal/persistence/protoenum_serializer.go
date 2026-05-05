package persistence

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"gorm.io/gorm/schema"
)

// ProtoEnumSerializer persists protobuf-generated `int32` enum types as plain
// integers, regardless of the underlying database driver's preference.
//
// Why this exists: protobuf's Go codegen produces enum types like
// `type VectorType int32` that also implement `fmt.Stringer` (returning the
// symbolic name, e.g. "VECTOR_TYPE_IMAGE"). The Postgres driver's default
// encoder for `database/sql/driver.Value` falls through to the Stringer for
// typed-int values whose underlying kind it does not natively recognize, so
// inserts into an `INTEGER` column fail with
//   ERROR: invalid input syntax for type integer: "VECTOR_TYPE_IMAGE"
// SQLite's driver hits the reflect-int path and is unaffected, which is why
// this only manifests on Postgres.
//
// Tagging the field with `serializer:protoenum` forces both Scan and Value
// to go through this type, which always reads/writes the column as an
// integer. Use only on protobuf-generated int32 enum fields.
type ProtoEnumSerializer struct{}

func init() {
	schema.RegisterSerializer("protoenum", ProtoEnumSerializer{})
}

// Scan reads an integer (or text-encoded integer, depending on the driver)
// from the database and assigns it to a typed-int32 enum field.
func (ProtoEnumSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	target := field.ReflectValueOf(ctx, dst)
	if !target.CanSet() {
		return fmt.Errorf("protoenum scan %s: field is not settable", field.Name)
	}
	if k := target.Kind(); k < reflect.Int || k > reflect.Int64 {
		return fmt.Errorf("protoenum scan %s: field kind %s is not an integer", field.Name, k)
	}

	switch v := dbValue.(type) {
	case nil:
		target.SetInt(0)
	case int64:
		target.SetInt(v)
	case int32:
		target.SetInt(int64(v))
	case int:
		target.SetInt(int64(v))
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return fmt.Errorf("protoenum scan %s from bytes %q: %w", field.Name, v, err)
		}
		target.SetInt(n)
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("protoenum scan %s from string %q: %w", field.Name, v, err)
		}
		target.SetInt(n)
	default:
		return fmt.Errorf("protoenum scan %s: unsupported db value type %T", field.Name, dbValue)
	}
	return nil
}

// Value extracts the underlying integer value from a protobuf enum so the
// driver writes it as an integer rather than going through the Stringer
// fallback path.
func (ProtoEnumSerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue any) (any, error) {
	if fieldValue == nil {
		return int64(0), nil
	}
	rv := reflect.ValueOf(fieldValue)
	if k := rv.Kind(); k < reflect.Int || k > reflect.Int64 {
		return nil, fmt.Errorf("protoenum value %s: type %T is not an integer", field.Name, fieldValue)
	}
	return rv.Int(), nil
}

// Compile-time guard so the package fails loudly if the GORM serializer
// interface signature ever drifts.
var _ schema.SerializerInterface = ProtoEnumSerializer{}
