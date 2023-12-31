package compile

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// This asm file implements a human-readable/writable form of a compiled
// program. This is mostly to support testing of the VM without going through
// the parsing and name resolution phases of a higher-level language. A
// disassembler is also implemented.
//
// The assembly format looks like this (indentation and spacing is arbitrary,
// but order of sections is important):
//
// 	program: +opt -opt                   # required, boolean options can be set/unset (e.g. "+recursion")
// 		loads:                             # optional, list of Loads
// 			name_of_load
// 		names:														 # optional, list of Names (attr/predeclared/universe)
//      fail
// 		globals:                           # optional, list of Globals
// 			x # 0 - comment can be used to indicate corresponding index
// 			y # 1
// 		constants:                         # optional, list of Constants
// 			string "abc"
// 			int    1234
// 			float  1.34
// 			bigint 9999999999999999999999999
// 			bytes  "xyz"
//
// 	function: NAME <stack> <params> <kwparams> +varargs +kwargs
//                                       # required at least once for top-level
//  	locals:                            # optional, list of Locals
// 			x
//  	cells:                             # optional, name in Locals that require cells
// 			x
// 		freevars:                          # optional, list of Freevars
// 			y
// 		catches:                           # optional, list of Catch blocks
// 			10 20 5                          # address of pc0-pc1 and startpc
// 		code:                              # required, list of instructions
//			NOP
// 			JMP 3
// 			CALL 2

var sections = map[string]bool{
	"program:":   true,
	"loads:":     true,
	"names:":     true,
	"globals:":   true,
	"constants:": true,
	"function:":  true,
	"locals:":    true,
	"cells:":     true,
	"freevars:":  true,
	"catches:":   true,
	"code:":      true,
}

// Asm loads a compiled program from its assembler textual format.
func Asm(b []byte) (*Program, error) {
	asm := asm{s: bufio.NewScanner(bytes.NewReader(b))}

	// must start with the program: section
	fields := asm.next()
	asm.program(fields)

	// optional sections
	fields = asm.next()
	fields = asm.loads(fields)
	fields = asm.names(fields)
	fields = asm.globals(fields)
	fields = asm.constants(fields)

	// functions
	for asm.err == nil && len(fields) > 0 && fields[0] == "function:" {
		fields = asm.function(fields)
	}

	if asm.err == nil {
		if len(fields) > 0 {
			asm.err = fmt.Errorf("unexpected section: %s", fields[0])
		} else if asm.p.Toplevel == nil {
			asm.err = errors.New("missing top-level function")
		}
	}
	return asm.p, asm.err
}

type asm struct {
	s   *bufio.Scanner
	p   *Program
	fn  *Funcode // current function
	err error
}

func (a *asm) function(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "function:") {
		return fields
	}

	if len(fields) < 5 {
		a.err = fmt.Errorf("invalid function: want at least 5 fields: 'function: NAME <stack> <params> <kwparams> [+varargs +kwargs]', got %d fields (%s)", len(fields), strings.Join(fields, " "))
		// force going forward, otherwise it would still process that line
		fields = a.next()
		return fields
	}
	fn := Funcode{
		Prog:            a.p,
		Name:            fields[1],
		MaxStack:        int(a.int(fields[2])),
		NumParams:       int(a.int(fields[3])),
		NumKwonlyParams: int(a.int(fields[4])),
		HasVarargs:      a.option(fields[5:], "varargs"),
		HasKwargs:       a.option(fields[5:], "kwargs"),
	}
	a.fn = &fn

	// function sub-sections
	fields = a.next()
	fields = a.locals(fields)
	fields = a.cells(fields)
	fields = a.freevars(fields)
	fields = a.catches(fields)
	fields = a.code(fields)

	// TODO: validate that catch blocks point to valid addresses

	a.fn = nil
	if a.p.Toplevel == nil {
		a.p.Toplevel = &fn
	} else {
		a.p.Functions = append(a.p.Functions, &fn)
	}
	return fields
}

func (a *asm) code(fields []string) []string {
	if a.err != nil {
		return fields
	}
	if len(fields) == 0 || !strings.EqualFold(fields[0], "code:") {
		msg := "expected code section"
		if len(fields) > 0 {
			msg += ", found " + fields[0]
		}
		a.err = errors.New(msg)
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		op, ok := reverseLookupOpcode[strings.ToLower(fields[0])]
		if !ok {
			a.err = fmt.Errorf("invalid opcode: %s", fields[0])
			return fields
		}

		var arg uint32
		if op >= OpcodeArgMin {
			// an argument is required
			if len(fields) != 2 {
				a.err = fmt.Errorf("expected an argument for opcode %s, got %d fields", fields[0], len(fields))
				return fields
			}
			arg = uint32(a.uint(fields[1]))
		} else if len(fields) != 1 {
			a.err = fmt.Errorf("expected no argument for opcode %s, got %d fields", fields[0], len(fields))
			return fields
		}
		a.fn.Code = encodeInsn(a.fn.Code, op, arg)
	}
	return fields
}

func (a *asm) catches(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "catches:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		if len(fields) != 3 {
			a.err = fmt.Errorf("invalid catch: expected pc0, pc1 and startpc, got %d fields", len(fields))
			return fields
		}

		a.fn.Catches = append(a.fn.Catches, Catch{
			PC0:     uint32(a.uint(fields[0])),
			PC1:     uint32(a.uint(fields[1])),
			StartPC: uint32(a.uint(fields[2])),
		})
	}
	return fields
}

func (a *asm) freevars(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "freevars:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		a.fn.Freevars = append(a.fn.Freevars, Binding{Name: fields[0]})
	}
	return fields
}

func (a *asm) cells(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "cells:") {
		return fields
	}

outer:
	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		for i, l := range a.fn.Locals {
			if l.Name == fields[0] {
				a.fn.Cells = append(a.fn.Cells, i)
				continue outer
			}
		}
		a.err = fmt.Errorf("invalid cell: %q is not an existing local", fields[0])
		return fields
	}
	return fields
}

func (a *asm) locals(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "locals:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		a.fn.Locals = append(a.fn.Locals, Binding{Name: fields[0]})
	}
	return fields
}

func (a *asm) constants(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "constants:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		if len(fields) != 2 {
			a.err = fmt.Errorf("invalid constant: expected type and value, got %d fields", len(fields))
			return fields
		}

		switch fields[0] {
		case "int":
			a.p.Constants = append(a.p.Constants, a.int(fields[1]))
		case "float":
			f, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				a.err = fmt.Errorf("invalid float: %s: %w", fields[1], err)
				return fields
			}
			a.p.Constants = append(a.p.Constants, f)
		case "bigint":
			bi := big.NewInt(0)
			bi, ok := bi.SetString(fields[1], 10)
			if !ok {
				a.err = fmt.Errorf("invalid bigint: %s", fields[1])
				return fields
			}
			a.p.Constants = append(a.p.Constants, bi)
		case "string":
			s, err := strconv.Unquote(fields[1])
			if err != nil {
				a.err = fmt.Errorf("invalid string: %q: %w", fields[1], err)
				return fields
			}
			a.p.Constants = append(a.p.Constants, s)
		case "bytes":
			s, err := strconv.Unquote(fields[1])
			if err != nil {
				a.err = fmt.Errorf("invalid bytes: %q: %w", fields[1], err)
				return fields
			}
			a.p.Constants = append(a.p.Constants, Bytes(s))
		default:
			a.err = fmt.Errorf("invalid constant type: %s", fields[0])
			return fields
		}
	}
	return fields
}

func (a *asm) globals(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "globals:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		a.p.Globals = append(a.p.Globals, Binding{Name: fields[0]})
	}
	return fields
}

func (a *asm) names(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "names:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		a.p.Names = append(a.p.Names, fields[0])
	}
	return fields
}

func (a *asm) loads(fields []string) []string {
	if a.err != nil || len(fields) == 0 || !strings.EqualFold(fields[0], "loads:") {
		return fields
	}

	for fields = a.next(); len(fields) > 0 && !sections[fields[0]]; fields = a.next() {
		a.p.Loads = append(a.p.Loads, Binding{Name: fields[0]})
	}
	return fields
}

func (a *asm) program(fields []string) {
	if a.err != nil {
		return
	}
	if len(fields) == 0 || !strings.EqualFold(fields[0], "program:") {
		msg := "expected program section"
		if len(fields) > 0 {
			msg += ", found " + fields[0]
		}
		a.err = errors.New(msg)
		return
	}

	var p Program
	p.Recursion = a.option(fields[1:], "recursion")
	a.p = &p
}

func (a *asm) option(fields []string, opt string) bool {
	for _, fld := range fields {
		if fld == "+"+opt {
			return true
		}
		if fld == "-"+opt {
			break
		}
	}
	return false
}

func (a *asm) int(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		a.err = fmt.Errorf("invalid integer: %s: %w", s, err)
	}
	return i
}

func (a *asm) uint(s string) uint64 {
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		a.err = fmt.Errorf("invalid unsigned integer: %s: %w", s, err)
	}
	return u
}

// returns the fields for the next non-empty, non-comment-only line, so that
// fields[0] will contain the line identification if it is a section.
func (a *asm) next() []string {
	if a.err != nil {
		return nil
	}
	for a.s.Scan() {
		line := a.s.Text()
		fields := strings.Fields(line)
		if len(fields) != 0 && !strings.HasPrefix(fields[0], "#") {
			// strip comments to make rest of parsing simpler
			for i, fld := range fields {
				if strings.HasPrefix(fld, "#") {
					fields = fields[:i]
					break
				}
			}
			return fields
		}
	}
	a.err = a.s.Err()
	return nil
}

// Dasm writes a compiled program to its assembler textual format.
func Dasm(p *Program) ([]byte, error) {
	panic("unreachable")
}
