package zapcolors

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/uber-go/zap"
)

const initialBufSize = 4096

var textPool = sync.Pool{New: func() interface{} {
	return &textEncoder{
		bytes: make([]byte, 0, initialBufSize),
	}
}}

type textEncoder struct {
	bytes       []byte
	timeFmt     string
	firstNested bool
}

// NewTextEncoder creates a line-oriented text encoder whose output is optimized
// for human, rather than machine, consumption. By default, the encoder uses
// RFC3339-formatted timestamps.
func NewColorEncoder(options ...TextOption) zap.Encoder {
	enc := textPool.Get().(*textEncoder)
	enc.truncate()
	enc.timeFmt = time.RFC3339
	for _, opt := range options {
		opt.apply(enc)
	}
	return enc
}

func (enc *textEncoder) Free() {
	textPool.Put(enc)
}

func (enc *textEncoder) AddString(key, val string) {
	enc.addKey(key)
	enc.bytes = append(enc.bytes, val...)
}

func (enc *textEncoder) AddBool(key string, val bool) {
	enc.addKey(key)
	enc.bytes = strconv.AppendBool(enc.bytes, val)
}

func (enc *textEncoder) AddInt(key string, val int) {
	enc.AddInt64(key, int64(val))
}

func (enc *textEncoder) AddInt64(key string, val int64) {
	enc.addKey(key)
	enc.bytes = strconv.AppendInt(enc.bytes, val, 10)
}

func (enc *textEncoder) AddUint(key string, val uint) {
	enc.AddUint64(key, uint64(val))
}

func (enc *textEncoder) AddUint64(key string, val uint64) {
	enc.addKey(key)
	enc.bytes = strconv.AppendUint(enc.bytes, val, 10)
}

func (enc *textEncoder) AddUintptr(key string, val uintptr) {
	enc.addKey(key)
	enc.bytes = append(enc.bytes, "0x"...)
	enc.bytes = strconv.AppendUint(enc.bytes, uint64(val), 16)
}

func (enc *textEncoder) AddFloat64(key string, val float64) {
	enc.addKey(key)
	enc.bytes = strconv.AppendFloat(enc.bytes, val, 'f', -1, 64)
}

func (enc *textEncoder) AddMarshaler(key string, obj zap.LogMarshaler) error {
	enc.addKey(key)
	enc.firstNested = true
	enc.bytes = append(enc.bytes, '{')
	err := obj.MarshalLog(enc)
	enc.bytes = append(enc.bytes, '}')
	enc.firstNested = false
	return err
}

func (enc *textEncoder) AddObject(key string, obj interface{}) error {
	enc.AddString(key, fmt.Sprintf("%+v", obj))
	return nil
}

func (enc *textEncoder) Clone() zap.Encoder {
	clone := textPool.Get().(*textEncoder)
	clone.truncate()
	clone.bytes = append(clone.bytes, enc.bytes...)
	clone.timeFmt = enc.timeFmt
	clone.firstNested = enc.firstNested
	return clone
}

func (enc *textEncoder) WriteEntry(sink io.Writer, msg string, lvl zap.Level, t time.Time) error {
	if sink == nil {
		return errors.New("NIL SINK ERR - wut")
	}

	final := textPool.Get().(*textEncoder)
	final.truncate()
	enc.addLevel(final, lvl)
	enc.addTime(final, t)
	enc.addMessage(final, msg)

	if len(enc.bytes) > 0 {
		final.bytes = append(final.bytes, ' ')
		final.bytes = append(final.bytes, enc.bytes...)
	}
	final.bytes = append(final.bytes, '\n')

	expectedBytes := len(final.bytes)
	n, err := sink.Write(final.bytes)
	final.Free()
	if err != nil {
		return err
	}
	if n != expectedBytes {
		return fmt.Errorf("incomplete write: only wrote %v of %v bytes", n, expectedBytes)
	}
	return nil
}

func (enc *textEncoder) truncate() {
	enc.bytes = enc.bytes[:0]
}

func (enc *textEncoder) addKey(key string) {
	lastIdx := len(enc.bytes) - 1
	if lastIdx >= 0 && !enc.firstNested {
		enc.bytes = append(enc.bytes, ' ')
	} else {
		enc.firstNested = false
	}
	
	var sum int
	for _, c := range []byte(key) {
		sum += int(c)	
	}
	
	color := (sum % 7) + 1
	
	enc.bytes = append(enc.bytes, []byte(fmt.Sprintf("\x1b[3%d;1m%s\x1b[0m", color, key))...)
	enc.bytes = append(enc.bytes, '=')
}

func (enc *textEncoder) addLevel(final *textEncoder, lvl zap.Level) {
	switch lvl {
	case zap.DebugLevel:
		final.bytes = append(final.bytes, []byte("\x1b[32;1m[DEBG]\x1b[0m")...)
	case zap.InfoLevel:
		final.bytes = append(final.bytes, []byte("\x1b[34;1m[INFO]\x1b[0m")...)
	case zap.WarnLevel:
		final.bytes = append(final.bytes, []byte("\x1b[33;1m[WARN]\x1b[0m")...)
	case zap.ErrorLevel:
		final.bytes = append(final.bytes, []byte("\x1b[31;1m[ERRO]\x1b[0m")...)
	case zap.PanicLevel:
		final.bytes = append(final.bytes, []byte("\x1b[31;1m[PANC]\x1b[0m")...)
	case zap.FatalLevel:
		final.bytes = append(final.bytes, []byte("\x1b[31;1m[FATA]\x1b[0m")...)
	default:
		final.bytes = strconv.AppendInt(final.bytes, int64(lvl), 10)
	}
}

func (enc *textEncoder) addTime(final *textEncoder, t time.Time) {
	if enc.timeFmt == "" {
		return
	}
	final.bytes = append(final.bytes, ' ')
	final.bytes = t.AppendFormat(final.bytes, enc.timeFmt)
}

func (enc *textEncoder) addMessage(final *textEncoder, msg string) {
	if msg != "" {
		final.bytes = append(final.bytes, ' ')
		final.bytes = append(final.bytes, []byte(fmt.Sprintf("%-25s", msg))...)
	}
}

// A TextOption is used to set options for a text encoder.
type TextOption interface {
	apply(*textEncoder)
}

type textOptionFunc func(*textEncoder)

func (opt textOptionFunc) apply(enc *textEncoder) {
	opt(enc)
}

// TextTimeFormat sets the format for log timestamps, using the same layout
// strings supported by time.Parse.
func TextTimeFormat(layout string) TextOption {
	return textOptionFunc(func(enc *textEncoder) {
		enc.timeFmt = layout
	})
}

// TextNoTime omits timestamps from the serialized log entries.
func TextNoTime() TextOption {
	return TextTimeFormat("")
}
