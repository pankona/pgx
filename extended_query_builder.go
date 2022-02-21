package pgx

import (
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"
)

type extendedQueryBuilder struct {
	paramValues     [][]byte
	paramValueBytes []byte
	paramFormats    []int16
	resultFormats   []int16
}

func (eqb *extendedQueryBuilder) AppendParam(m *pgtype.Map, oid uint32, arg interface{}) error {
	f := chooseParameterFormatCode(m, oid, arg)
	eqb.paramFormats = append(eqb.paramFormats, f)

	v, err := eqb.encodeExtendedParamValue(m, oid, f, arg)
	if err != nil {
		return err
	}
	eqb.paramValues = append(eqb.paramValues, v)

	return nil
}

func (eqb *extendedQueryBuilder) AppendResultFormat(f int16) {
	eqb.resultFormats = append(eqb.resultFormats, f)
}

// Reset readies eqb to build another query.
func (eqb *extendedQueryBuilder) Reset() {
	eqb.paramValues = eqb.paramValues[0:0]
	eqb.paramValueBytes = eqb.paramValueBytes[0:0]
	eqb.paramFormats = eqb.paramFormats[0:0]
	eqb.resultFormats = eqb.resultFormats[0:0]

	if cap(eqb.paramValues) > 64 {
		eqb.paramValues = make([][]byte, 0, 64)
	}

	if cap(eqb.paramValueBytes) > 256 {
		eqb.paramValueBytes = make([]byte, 0, 256)
	}

	if cap(eqb.paramFormats) > 64 {
		eqb.paramFormats = make([]int16, 0, 64)
	}
	if cap(eqb.resultFormats) > 64 {
		eqb.resultFormats = make([]int16, 0, 64)
	}
}

func (eqb *extendedQueryBuilder) encodeExtendedParamValue(m *pgtype.Map, oid uint32, formatCode int16, arg interface{}) ([]byte, error) {
	if arg == nil {
		return nil, nil
	}

	refVal := reflect.ValueOf(arg)
	argIsPtr := refVal.Kind() == reflect.Ptr

	if argIsPtr && refVal.IsNil() {
		return nil, nil
	}

	if eqb.paramValueBytes == nil {
		eqb.paramValueBytes = make([]byte, 0, 128)
	}

	pos := len(eqb.paramValueBytes)

	if arg, ok := arg.(string); ok {
		return []byte(arg), nil
	}

	if argIsPtr {
		// We have already checked that arg is not pointing to nil,
		// so it is safe to dereference here.
		arg = refVal.Elem().Interface()
		return eqb.encodeExtendedParamValue(m, oid, formatCode, arg)
	}

	if _, ok := m.TypeForOID(oid); ok {
		buf, err := m.Encode(oid, formatCode, arg, eqb.paramValueBytes)
		if err != nil {
			return nil, err
		}
		if buf == nil {
			return nil, nil
		}
		eqb.paramValueBytes = buf
		return eqb.paramValueBytes[pos:], nil
	}

	if strippedArg, ok := stripNamedType(&refVal); ok {
		return eqb.encodeExtendedParamValue(m, oid, formatCode, strippedArg)
	}
	return nil, SerializationError(fmt.Sprintf("Cannot encode %T into oid %v - %T must implement Encoder or be converted to a string", arg, oid, arg))
}
