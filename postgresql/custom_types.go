// Copyright (c) 2012-present The upper.io/db authors. All rights reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
// LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
// OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
// WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package postgresql

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	stateInit = iota
	stateOpenBracket
	stateOpenQuote
	stateLiteral
	stateEscape
	stateStop
)

// Type JSONB represents a PostgreSQL's JSONB column.
type JSONB struct {
	V interface{}
}

// Scan implements the sql.Scanner interface.
func (j *JSONB) Scan(src interface{}) error {
	if src == nil {
		j.V = nil
		return nil
	}

	b, ok := src.([]byte)
	if !ok {
		return errors.New("Scan source was not []bytes")
	}

	v := JSONB{}
	if err := json.Unmarshal(b, &v.V); err != nil {
		return err
	}
	*j = v
	return nil
}

// Value implements the driver.Valuer interface.
func (j JSONB) Value() (driver.Value, error) {
	if j.V == nil {
		return nil, nil
	}
	if v, ok := j.V.(json.RawMessage); ok {
		return string(v), nil
	}
	return json.Marshal(j.V)
}

// StringArray represents a PostgreSQL's varchar array.
type StringArray []string

// Scan implements the sql.Scanner interface.
func (a *StringArray) Scan(src interface{}) error {
	if src == nil {
		*a = nil
		return nil
	}

	b, ok := src.([]byte)
	if !ok {
		return errors.New("Scan source was not []bytes")
	}
	if len(b) == 0 {
		*a = nil
		return nil
	}

	results := []string{}

	state := stateOpenBracket
	var buffer []byte

	for i := 1; i < len(b); i++ {
		c := b[i]

		switch state {
		case stateStop:
			return fmt.Errorf("Got additional data beyond expected bounds")
		case stateInit:
			switch c {
			case '{':
				buffer = nil
				state = stateOpenBracket
			default:
				return fmt.Errorf("Expecting { at position %d", i)
			}
		case stateOpenBracket:
			switch c {
			case '}':
				if buffer != nil {
					results = append(results, string(buffer))
				}
				state = stateStop
				break
			case ' ':
				continue
			case ',':
				results = append(results, string(buffer))
				buffer = []byte{}
				continue
			case '"':
				state = stateOpenQuote
				buffer = []byte{}
			default:
				state = stateLiteral
				buffer = []byte{c}
			}
		case stateLiteral:
			switch c {
			case '}':
				results = append(results, string(buffer))
				state = stateStop
			case ',':
				results = append(results, string(buffer))
				buffer = []byte{}

				state = stateOpenBracket
			default:
				buffer = append(buffer, c)
			}
		case stateEscape:
			buffer = append(buffer, c)
			state = stateOpenQuote
		case stateOpenQuote:
			switch c {
			case '\\':
				state = stateEscape
				continue
			case '"':
				state = stateOpenBracket
			default:
				buffer = append(buffer, c)
			}
		}
	}

	*a = StringArray(results)
	return nil
}

// Value implements the driver.Valuer interface.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}

	if n := len(a); n > 0 {
		// There will be at least two curly brackets, 2*N bytes of quotes,
		// and N-1 bytes of delimiters.
		b := make([]byte, 1, 1+3*n)
		b[0] = '{'

		b = appendArrayQuotedString(b, a[0])
		for i := 1; i < n; i++ {
			b = append(b, ',')
			b = appendArrayQuotedString(b, a[i])
		}

		return append(b, '}'), nil
	}

	return []byte{'{', '}'}, nil
}

func appendArrayQuotedString(b []byte, v string) []byte {
	b = append(b, '"')
	for {
		i := strings.IndexAny(v, `"\`)
		if i < 0 {
			b = append(b, v...)
			break
		}
		if i > 0 {
			b = append(b, v[:i]...)
		}
		b = append(b, '\\', v[i])
		v = v[i+1:]
	}
	return append(b, '"')
}

// Int64Array represents a PostgreSQL's integer array.
type Int64Array []int64

// Scan implements the sql.Scanner interface.
func (a *Int64Array) Scan(src interface{}) error {
	if src == nil {
		*a = nil
		return nil
	}
	b, ok := src.([]byte)
	if !ok {
		return errors.New("Scan source was not []bytes")
	}
	if len(b) == 0 {
		*a = nil
		return nil
	}

	s := string(b)[1 : len(b)-1]
	results := []int64{}
	if s != "" {
		parts := strings.Split(s, ",")
		for _, n := range parts {
			i, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return err
			}
			results = append(results, i)
		}
	}
	*a = Int64Array(results)
	return nil
}

// Value implements the driver.Valuer interface.
func (a Int64Array) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}

	if n := len(a); n > 0 {
		// There will be at least two curly brackets, N bytes of values,
		// and N-1 bytes of delimiters.
		b := make([]byte, 1, 1+2*n)
		b[0] = '{'

		b = strconv.AppendInt(b, a[0], 10)
		for i := 1; i < n; i++ {
			b = append(b, ',')
			b = strconv.AppendInt(b, a[i], 10)
		}

		return append(b, '}'), nil
	}

	return []byte{'{', '}'}, nil
}
