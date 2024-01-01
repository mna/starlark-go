package compile_test

import (
	"testing"

	"github.com/mna/nenuphar/internal/compile"
	"github.com/stretchr/testify/require"
)

func TestAsm(t *testing.T) {
	cases := []struct {
		desc string
		in   string
		err  string // error "contains" this err string, no error if empty
	}{
		{"empty", ``, "expected program section"},
		{"not program", `function:`, "expected program section"},
		{"program only", `program: foo bar +baz`, "missing top-level function"},

		{"invalid function", `
				program:
					function: MissingNumArgs
						code:
			`, "invalid function: want at least 5 fields"},

		{"minimally valid", `
				program:
					function: Top 0 0 0
						code:
			`, ""},

		{"missing code", `
				program:
					function: Top 0 0 0
			`, "expected code section"},

		{"missing code followed by function", `
				program:
					function: Top 0 0 0
					function: Top 0 0 0
						code:
			`, "expected code section"},

		{"extra unknown section", `
				program:
					function: Top 0 0 0
						code:
				locals:
				`, "unexpected section: locals:"},

		{"invalid opcode", `
				program:
					function: Top 0 0 0
						code:
							foobar
				`, "invalid opcode: foobar"},

		{"missing opcode arg", `
				program:
					function: Top 0 0 0
						code:
							JMP
				`, "expected an argument for opcode JMP"},

		{"extra opcode arg", `
				program:
					function: Top 0 0 0
						code:
							JMP 1 2
				`, "expected an argument for opcode JMP, got 3 fields"},

		{"unexpected opcode arg", `
				program:
					function: Top 0 0 0
						code:
							NOP 1
				`, "expected no argument for opcode NOP"},

		{"invalid jump address", `
				program:
					function: Top 0 0 0
						code:
							NOP
							JMP 2
				`, "invalid jump index 2"},

		{"invalid catch number of fields", `
				program:
					function: Top 0 0 0
						catches:
							1
						code:
							NOP
				`, "invalid catch"},

		{"invalid catch not an integer", `
				program:
					function: Top 0 0 0
						catches:
							a b c
						code:
							NOP
				`, "invalid unsigned integer"},

		{"invalid catch address pc0", `
				program:
					function: Top 0 0 0
						catches:
							1 2 3
						code:
							NOP
				`, "invalid PC0 index 1"},

		{"invalid catch address pc1", `
				program:
					function: Top 0 0 0
						catches:
							0 2 3
						code:
							NOP
				`, "invalid PC1 index 2"},

		{"invalid catch address startpc", `
				program:
					function: Top 0 0 0
						catches:
							0 2 3
						code:
							NOP
							NOP
							NOP
				`, "invalid StartPC index 3"},

		{"invalid cell", `
				program:
					function: Top 0 0 0
						locals:
							x
							y
						cells:
							z
				`, "invalid cell"},

		{"invalid constant number of fields", `
				program:
					constants:
						123
				`, "invalid constant: expected type and value"},

		{"invalid constant type", `
				program:
					constants:
						foo 123
				`, "invalid constant type"},

		{"invalid integer constant", `
				program:
					constants:
						int abc
				`, "invalid integer"},

		{"invalid float constant", `
				program:
					constants:
						float abc
				`, "invalid float"},

		{"invalid bigint constant", `
				program:
					constants:
						bigint abc
				`, "invalid bigint"},

		{"invalid string constant", `
				program:
					constants:
						string "a'
				`, "invalid string"},

		{"invalid bytes constant", `
				program:
					constants:
						bytes "\x0"
				`, "invalid bytes"},

		{"maximally valid", `
				program: +recursion
					loads:
						math
						json
					names:
						name
						age
					globals:
						env
					constants:
						string "abc"
						int 1234
						float 3.1415
						bigint 98765432109876543210
						bytes "xyz"

					function: Top 1 0 0 +varargs
						locals:
							z
						cells:
							z
						code:
							NOP

					function: Nested 2 1 1 +kwargs
						locals:
							x
							y
						cells:
							x
						freevars:
							z
						catches:
							2 3 1
						code:
							TRUE
							DUP
							FALSE
							NOP
							JMP 1
			`, ""},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			_, err := compile.Asm([]byte(c.in))
			if c.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, c.err)
		})
	}
}
