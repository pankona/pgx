package pgtype

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgio"
)

const pgTimestamptzHourFormat = "2006-01-02 15:04:05.999999999Z07"
const pgTimestamptzMinuteFormat = "2006-01-02 15:04:05.999999999Z07:00"
const pgTimestamptzSecondFormat = "2006-01-02 15:04:05.999999999Z07:00:00"
const microsecFromUnixEpochToY2K = 946684800 * 1000000

const (
	negativeInfinityMicrosecondOffset = -9223372036854775808
	infinityMicrosecondOffset         = 9223372036854775807
)

type TimestamptzScanner interface {
	ScanTimestamptz(v Timestamptz) error
}

type TimestamptzValuer interface {
	TimestamptzValue() (Timestamptz, error)
}

// Timestamptz represents the PostgreSQL timestamptz type.
type Timestamptz struct {
	Time             time.Time
	InfinityModifier InfinityModifier
	Valid            bool
}

func (tstz *Timestamptz) ScanTimestamptz(v Timestamptz) error {
	*tstz = v
	return nil
}

func (tstz Timestamptz) TimestamptzValue() (Timestamptz, error) {
	return tstz, nil
}

// Scan implements the database/sql Scanner interface.
func (tstz *Timestamptz) Scan(src interface{}) error {
	if src == nil {
		*tstz = Timestamptz{}
		return nil
	}

	switch src := src.(type) {
	case string:
		return scanPlanTextTimestamptzToTimestamptzScanner{}.Scan([]byte(src), tstz)
	case time.Time:
		*tstz = Timestamptz{Time: src, Valid: true}
		return nil
	}

	return fmt.Errorf("cannot scan %T", src)
}

// Value implements the database/sql/driver Valuer interface.
func (tstz Timestamptz) Value() (driver.Value, error) {
	if !tstz.Valid {
		return nil, nil
	}

	if tstz.InfinityModifier != None {
		return tstz.InfinityModifier.String(), nil
	}
	return tstz.Time, nil
}

func (tstz Timestamptz) MarshalJSON() ([]byte, error) {
	if !tstz.Valid {
		return []byte("null"), nil
	}

	var s string

	switch tstz.InfinityModifier {
	case None:
		s = tstz.Time.Format(time.RFC3339Nano)
	case Infinity:
		s = "infinity"
	case NegativeInfinity:
		s = "-infinity"
	}

	return json.Marshal(s)
}

func (tstz *Timestamptz) UnmarshalJSON(b []byte) error {
	var s *string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	if s == nil {
		*tstz = Timestamptz{}
		return nil
	}

	switch *s {
	case "infinity":
		*tstz = Timestamptz{Valid: true, InfinityModifier: Infinity}
	case "-infinity":
		*tstz = Timestamptz{Valid: true, InfinityModifier: -Infinity}
	default:
		// PostgreSQL uses ISO 8601 for to_json function and casting from a string to timestamptz
		tim, err := time.Parse(time.RFC3339Nano, *s)
		if err != nil {
			return err
		}

		*tstz = Timestamptz{Time: tim, Valid: true}
	}

	return nil
}

type TimestamptzCodec struct{}

func (TimestamptzCodec) FormatSupported(format int16) bool {
	return format == TextFormatCode || format == BinaryFormatCode
}

func (TimestamptzCodec) PreferredFormat() int16 {
	return BinaryFormatCode
}

func (TimestamptzCodec) PlanEncode(m *Map, oid uint32, format int16, value interface{}) EncodePlan {
	if _, ok := value.(TimestamptzValuer); !ok {
		return nil
	}

	switch format {
	case BinaryFormatCode:
		return encodePlanTimestamptzCodecBinary{}
	case TextFormatCode:
		return encodePlanTimestamptzCodecText{}
	}

	return nil
}

type encodePlanTimestamptzCodecBinary struct{}

func (encodePlanTimestamptzCodecBinary) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	ts, err := value.(TimestamptzValuer).TimestamptzValue()
	if err != nil {
		return nil, err
	}

	if !ts.Valid {
		return nil, nil
	}

	var microsecSinceY2K int64
	switch ts.InfinityModifier {
	case None:
		microsecSinceUnixEpoch := ts.Time.Unix()*1000000 + int64(ts.Time.Nanosecond())/1000
		microsecSinceY2K = microsecSinceUnixEpoch - microsecFromUnixEpochToY2K
	case Infinity:
		microsecSinceY2K = infinityMicrosecondOffset
	case NegativeInfinity:
		microsecSinceY2K = negativeInfinityMicrosecondOffset
	}

	buf = pgio.AppendInt64(buf, microsecSinceY2K)

	return buf, nil
}

type encodePlanTimestamptzCodecText struct{}

func (encodePlanTimestamptzCodecText) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	ts, err := value.(TimestamptzValuer).TimestamptzValue()
	if err != nil {
		return nil, err
	}

	var s string

	switch ts.InfinityModifier {
	case None:
		s = ts.Time.UTC().Truncate(time.Microsecond).Format(pgTimestamptzSecondFormat)
	case Infinity:
		s = "infinity"
	case NegativeInfinity:
		s = "-infinity"
	}

	buf = append(buf, s...)

	return buf, nil
}

func (TimestamptzCodec) PlanScan(m *Map, oid uint32, format int16, target interface{}, actualTarget bool) ScanPlan {

	switch format {
	case BinaryFormatCode:
		switch target.(type) {
		case TimestamptzScanner:
			return scanPlanBinaryTimestamptzToTimestamptzScanner{}
		}
	case TextFormatCode:
		switch target.(type) {
		case TimestamptzScanner:
			return scanPlanTextTimestamptzToTimestamptzScanner{}
		}
	}

	return nil
}

type scanPlanBinaryTimestamptzToTimestamptzScanner struct{}

func (scanPlanBinaryTimestamptzToTimestamptzScanner) Scan(src []byte, dst interface{}) error {
	scanner := (dst).(TimestamptzScanner)

	if src == nil {
		return scanner.ScanTimestamptz(Timestamptz{})
	}

	if len(src) != 8 {
		return fmt.Errorf("invalid length for timestamptz: %v", len(src))
	}

	var tstz Timestamptz
	microsecSinceY2K := int64(binary.BigEndian.Uint64(src))

	switch microsecSinceY2K {
	case infinityMicrosecondOffset:
		tstz = Timestamptz{Valid: true, InfinityModifier: Infinity}
	case negativeInfinityMicrosecondOffset:
		tstz = Timestamptz{Valid: true, InfinityModifier: -Infinity}
	default:
		tim := time.Unix(
			microsecFromUnixEpochToY2K/1000000+microsecSinceY2K/1000000,
			(microsecFromUnixEpochToY2K%1000000*1000)+(microsecSinceY2K%1000000*1000),
		)
		tstz = Timestamptz{Time: tim, Valid: true}
	}

	return scanner.ScanTimestamptz(tstz)
}

type scanPlanTextTimestamptzToTimestamptzScanner struct{}

func (scanPlanTextTimestamptzToTimestamptzScanner) Scan(src []byte, dst interface{}) error {
	scanner := (dst).(TimestamptzScanner)

	if src == nil {
		return scanner.ScanTimestamptz(Timestamptz{})
	}

	var tstz Timestamptz
	sbuf := string(src)
	switch sbuf {
	case "infinity":
		tstz = Timestamptz{Valid: true, InfinityModifier: Infinity}
	case "-infinity":
		tstz = Timestamptz{Valid: true, InfinityModifier: -Infinity}
	default:
		var format string
		if len(sbuf) >= 9 && (sbuf[len(sbuf)-9] == '-' || sbuf[len(sbuf)-9] == '+') {
			format = pgTimestamptzSecondFormat
		} else if len(sbuf) >= 6 && (sbuf[len(sbuf)-6] == '-' || sbuf[len(sbuf)-6] == '+') {
			format = pgTimestamptzMinuteFormat
		} else {
			format = pgTimestamptzHourFormat
		}

		tim, err := time.Parse(format, sbuf)
		if err != nil {
			return err
		}

		tstz = Timestamptz{Time: tim, Valid: true}
	}

	return scanner.ScanTimestamptz(tstz)
}

func (c TimestamptzCodec) DecodeDatabaseSQLValue(m *Map, oid uint32, format int16, src []byte) (driver.Value, error) {
	if src == nil {
		return nil, nil
	}

	var tstz Timestamptz
	err := codecScan(c, m, oid, format, src, &tstz)
	if err != nil {
		return nil, err
	}

	if tstz.InfinityModifier != None {
		return tstz.InfinityModifier.String(), nil
	}

	return tstz.Time, nil
}

func (c TimestamptzCodec) DecodeValue(m *Map, oid uint32, format int16, src []byte) (interface{}, error) {
	if src == nil {
		return nil, nil
	}

	var tstz Timestamptz
	err := codecScan(c, m, oid, format, src, &tstz)
	if err != nil {
		return nil, err
	}

	if tstz.InfinityModifier != None {
		return tstz.InfinityModifier, nil
	}

	return tstz.Time, nil
}
