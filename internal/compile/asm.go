package compile

import (
	"bufio"
	"bytes"
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

	// top-level function, required
	fields = asm.function(fields)
	for len(fields) > 0 && fields[0] == "function:" {
		// other functions
		fields = asm.function(fields)
	}

	return asm.p, asm.err
}

type asm struct {
	s   *bufio.Scanner
	p   *Program
	err error
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
			i, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				a.err = fmt.Errorf("invalid integer constant: %s: %w", fields[1], err)
				return fields
			}
			a.p.Constants = append(a.p.Constants, i)
		case "float":
			f, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				a.err = fmt.Errorf("invalid float constant: %s: %w", fields[1], err)
				return fields
			}
			a.p.Constants = append(a.p.Constants, f)
		case "bigint":
			bi := big.NewInt(0)
			bi, ok := bi.SetString(fields[1], 10)
			if !ok {
				a.err = fmt.Errorf("invalid bigint constant: %s", fields[1])
				return fields
			}
			a.p.Constants = append(a.p.Constants, bi)
		case "string":
			a.p.Constants = append(a.p.Constants, fields[1])
		case "bytes":
			a.p.Constants = append(a.p.Constants, Bytes(fields[1]))
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
		a.err = fmt.Errorf("expected program section, found %s", fields[0])
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
