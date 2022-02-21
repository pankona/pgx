package pgtype

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
)

type BoolScanner interface {
	ScanBool(v Bool) error
}

type BoolValuer interface {
	BoolValue() (Bool, error)
}

type Bool struct {
	Bool  bool
	Valid bool
}

func (b *Bool) ScanBool(v Bool) error {
	*b = v
	return nil
}

func (b Bool) BoolValue() (Bool, error) {
	return b, nil
}

// Scan implements the database/sql Scanner interface.
func (dst *Bool) Scan(src interface{}) error {
	if src == nil {
		*dst = Bool{}
		return nil
	}

	switch src := src.(type) {
	case bool:
		*dst = Bool{Bool: src, Valid: true}
		return nil
	case string:
		b, err := strconv.ParseBool(src)
		if err != nil {
			return err
		}
		*dst = Bool{Bool: b, Valid: true}
		return nil
	case []byte:
		b, err := strconv.ParseBool(string(src))
		if err != nil {
			return err
		}
		*dst = Bool{Bool: b, Valid: true}
		return nil
	}

	return fmt.Errorf("cannot scan %T", src)
}

// Value implements the database/sql/driver Valuer interface.
func (src Bool) Value() (driver.Value, error) {
	if !src.Valid {
		return nil, nil
	}

	return src.Bool, nil
}

func (src Bool) MarshalJSON() ([]byte, error) {
	if !src.Valid {
		return []byte("null"), nil
	}

	if src.Bool {
		return []byte("true"), nil
	} else {
		return []byte("false"), nil
	}
}

func (dst *Bool) UnmarshalJSON(b []byte) error {
	var v *bool
	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}

	if v == nil {
		*dst = Bool{}
	} else {
		*dst = Bool{Bool: *v, Valid: true}
	}

	return nil
}

type BoolCodec struct{}

func (BoolCodec) FormatSupported(format int16) bool {
	return format == TextFormatCode || format == BinaryFormatCode
}

func (BoolCodec) PreferredFormat() int16 {
	return BinaryFormatCode
}

func (BoolCodec) PlanEncode(m *Map, oid uint32, format int16, value interface{}) EncodePlan {
	switch format {
	case BinaryFormatCode:
		switch value.(type) {
		case bool:
			return encodePlanBoolCodecBinaryBool{}
		case BoolValuer:
			return encodePlanBoolCodecBinaryBoolValuer{}
		}
	case TextFormatCode:
		switch value.(type) {
		case bool:
			return encodePlanBoolCodecTextBool{}
		case BoolValuer:
			return encodePlanBoolCodecTextBoolValuer{}
		}
	}

	return nil
}

type encodePlanBoolCodecBinaryBool struct{}

func (encodePlanBoolCodecBinaryBool) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	v := value.(bool)

	if v {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}

	return buf, nil
}

type encodePlanBoolCodecTextBoolValuer struct{}

func (encodePlanBoolCodecTextBoolValuer) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	b, err := value.(BoolValuer).BoolValue()
	if err != nil {
		return nil, err
	}

	if !b.Valid {
		return nil, nil
	}

	if b.Bool {
		buf = append(buf, 't')
	} else {
		buf = append(buf, 'f')
	}

	return buf, nil
}

type encodePlanBoolCodecBinaryBoolValuer struct{}

func (encodePlanBoolCodecBinaryBoolValuer) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	b, err := value.(BoolValuer).BoolValue()
	if err != nil {
		return nil, err
	}

	if !b.Valid {
		return nil, nil
	}

	if b.Bool {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}

	return buf, nil
}

type encodePlanBoolCodecTextBool struct{}

func (encodePlanBoolCodecTextBool) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	v := value.(bool)

	if v {
		buf = append(buf, 't')
	} else {
		buf = append(buf, 'f')
	}

	return buf, nil
}

func (BoolCodec) PlanScan(m *Map, oid uint32, format int16, target interface{}, actualTarget bool) ScanPlan {

	switch format {
	case BinaryFormatCode:
		switch target.(type) {
		case *bool:
			return scanPlanBinaryBoolToBool{}
		case BoolScanner:
			return scanPlanBinaryBoolToBoolScanner{}
		}
	case TextFormatCode:
		switch target.(type) {
		case *bool:
			return scanPlanTextAnyToBool{}
		case BoolScanner:
			return scanPlanTextAnyToBoolScanner{}
		}
	}

	return nil
}

func (c BoolCodec) DecodeDatabaseSQLValue(m *Map, oid uint32, format int16, src []byte) (driver.Value, error) {
	return c.DecodeValue(m, oid, format, src)
}

func (c BoolCodec) DecodeValue(m *Map, oid uint32, format int16, src []byte) (interface{}, error) {
	if src == nil {
		return nil, nil
	}

	var b bool
	err := codecScan(c, m, oid, format, src, &b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

type scanPlanBinaryBoolToBool struct{}

func (scanPlanBinaryBoolToBool) Scan(src []byte, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("cannot scan null into %T", dst)
	}

	if len(src) != 1 {
		return fmt.Errorf("invalid length for bool: %v", len(src))
	}

	p, ok := (dst).(*bool)
	if !ok {
		return ErrScanTargetTypeChanged
	}

	*p = src[0] == 1

	return nil
}

type scanPlanTextAnyToBool struct{}

func (scanPlanTextAnyToBool) Scan(src []byte, dst interface{}) error {
	if src == nil {
		return fmt.Errorf("cannot scan null into %T", dst)
	}

	if len(src) != 1 {
		return fmt.Errorf("invalid length for bool: %v", len(src))
	}

	p, ok := (dst).(*bool)
	if !ok {
		return ErrScanTargetTypeChanged
	}

	*p = src[0] == 't'

	return nil
}

type scanPlanBinaryBoolToBoolScanner struct{}

func (scanPlanBinaryBoolToBoolScanner) Scan(src []byte, dst interface{}) error {
	s, ok := (dst).(BoolScanner)
	if !ok {
		return ErrScanTargetTypeChanged
	}

	if src == nil {
		return s.ScanBool(Bool{})
	}

	if len(src) != 1 {
		return fmt.Errorf("invalid length for bool: %v", len(src))
	}

	return s.ScanBool(Bool{Bool: src[0] == 1, Valid: true})
}

type scanPlanTextAnyToBoolScanner struct{}

func (scanPlanTextAnyToBoolScanner) Scan(src []byte, dst interface{}) error {
	s, ok := (dst).(BoolScanner)
	if !ok {
		return ErrScanTargetTypeChanged
	}

	if src == nil {
		return s.ScanBool(Bool{})
	}

	if len(src) != 1 {
		return fmt.Errorf("invalid length for bool: %v", len(src))
	}

	return s.ScanBool(Bool{Bool: src[0] == 't', Valid: true})
}
