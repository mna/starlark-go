// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/mna/nenuphar/syntax"
)

// Int is the type of a Starlark int.
type Int int64

var _ HasUnary = Int(0)

// Unary implements the operations +int, -int, and ~int.
func (i Int) Unary(op syntax.Token) (Value, error) {
	switch op {
	case syntax.MINUS:
		return -i, nil
	case syntax.PLUS:
		return i, nil
	case syntax.TILDE:
		return ^i, nil
	}
	return nil, nil
}

func (i Int) String() string {
	return strconv.FormatInt(int64(i), 10)
}

func (i Int) Type() string { return "int" }
func (i Int) Freeze()      {}                // immutable
func (i Int) Truth() Bool  { return i != 0 } // true if non-zero
func (i Int) Hash() (uint32, error) {
	// TODO(mna): needs some consideration, would that even be needed if using
	// Golang's native map?
	return 12582917 * uint32(i+3), nil
}

// Cmp implements comparison of two Int values.
// Required by the TotallyOrdered interface.
func (i Int) Cmp(v Value, depth int) (int, error) {
	j := v.(Int)
	return int(i - j), nil // TODO: over/underflow on 32-bit platforms
}

// AsInt32 returns the value of x if is representable as an int32.
func AsInt32(x Value) (int, error) {
	i, ok := x.(Int)
	if !ok {
		return 0, fmt.Errorf("got %s, want int", x.Type())
	}
	if i < math.MinInt32 || i > math.MaxInt32 {
		return 0, fmt.Errorf("%s out of range", i)
	}
	return int(i), nil
}

// AsInt sets *ptr to the value of Starlark int x, if it is exactly representable,
// otherwise it returns an error.
// The type of ptr must be one of the pointer types *int, *int8, *int16, *int32, or *int64,
// or one of their unsigned counterparts including *uintptr.
func AsInt(x Value, ptr any) error {
	i, ok := x.(Int)
	if !ok {
		return fmt.Errorf("got %s, want int", x.Type())
	}

	bits := reflect.TypeOf(ptr).Elem().Size() * 8
	switch ptr.(type) {
	case *int, *int8, *int16, *int32, *int64:
		if bits < 64 && !(-1<<(bits-1) <= i && i < 1<<(bits-1)) {
			return fmt.Errorf("%s out of range (want value in signed %d-bit range)", i, bits)
		}
		switch ptr := ptr.(type) {
		case *int:
			*ptr = int(i)
		case *int8:
			*ptr = int8(i)
		case *int16:
			*ptr = int16(i)
		case *int32:
			*ptr = int32(i)
		case *int64:
			*ptr = int64(i)
		}

	case *uint, *uint8, *uint16, *uint32, *uint64, *uintptr:
		if i < 0 || bits < 64 && i >= 1<<bits {
			return fmt.Errorf("%s out of range (want value in unsigned %d-bit range)", i, bits)
		}
		switch ptr := ptr.(type) {
		case *uint:
			*ptr = uint(i)
		case *uint8:
			*ptr = uint8(i)
		case *uint16:
			*ptr = uint16(i)
		case *uint32:
			*ptr = uint32(i)
		case *uint64:
			*ptr = uint64(i)
		case *uintptr:
			*ptr = uintptr(i)
		}
	default:
		panic(fmt.Sprintf("invalid argument type: %T", ptr))
	}
	return nil
}

// NumberToInt converts a number x to an integer value.
// An int is returned unchanged, a float is truncated towards zero.
// NumberToInt reports an error for all other values.
func NumberToInt(x Value) (Int, error) {
	switch x := x.(type) {
	case Int:
		return x, nil
	case Float:
		f := float64(x)
		if math.IsInf(f, 0) {
			return 0, fmt.Errorf("cannot convert float infinity to integer")
		} else if math.IsNaN(f) {
			return 0, fmt.Errorf("cannot convert float NaN to integer")
		}
		return finiteFloatToInt(x), nil

	}
	return 0, fmt.Errorf("cannot convert %s to int", x.Type())
}

// finiteFloatToInt converts f to an Int, truncating towards zero.
func finiteFloatToInt(f Float) Int {
	return Int(f)
}
