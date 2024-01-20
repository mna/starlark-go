package compile_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mna/nenuphar/internal/chunkedfile"
	"github.com/mna/nenuphar/internal/compile"
	"github.com/mna/nenuphar/starlark"
	"github.com/mna/nenuphar/starlarkstruct"
	"github.com/mna/nenuphar/syntax"
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

					function: Defer 2 1 1 +varargs
						locals:
							x
						defers:
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

func TestDasm(t *testing.T) {
	cases := []struct {
		desc string
		p    compile.Program
		err  string // error "contains" this err string, no error if empty
	}{
		{"empty", compile.Program{}, "missing top-level function"},

		{"invalid constant type", compile.Program{
			Toplevel:  &compile.Funcode{},
			Constants: []any{true},
		}, "unsupported constant type: bool"},

		{"invalid opcode argument", compile.Program{
			Toplevel: &compile.Funcode{
				Code: []byte{byte(compile.JMP), '\xff', '\xff', '\xff', '\xff', '\xff', '\x00'},
			},
		}, "invalid uvarint argument"},

		{"invalid catch.pc0", compile.Program{
			Toplevel: &compile.Funcode{
				Code:    []byte{byte(compile.NOP), byte(compile.NOP)},
				Catches: []compile.Defer{{PC0: 2, PC1: 3, StartPC: 0}},
			},
		}, "invalid catch.pc0 address"},

		{"invalid catch.pc1", compile.Program{
			Toplevel: &compile.Funcode{
				Code:    []byte{byte(compile.JMP), '\xff', '\x00', byte(compile.NOP)},
				Catches: []compile.Defer{{PC0: 0, PC1: 1, StartPC: 3}},
			},
		}, "invalid catch.pc1 address"},

		{"invalid catch.startpc", compile.Program{
			Toplevel: &compile.Funcode{
				Code:    []byte{byte(compile.JMP), '\xff', '\x00', '\x00', '\x00', byte(compile.NOP)},
				Catches: []compile.Defer{{PC0: 0, PC1: 5, StartPC: 2}},
			},
		}, "invalid catch.startpc address"},

		{"invalid jump", compile.Program{
			Toplevel: &compile.Funcode{
				Code: []byte{byte(compile.JMP), '\x02', '\x00', '\x00', '\x00', byte(compile.NOP)},
			},
		}, "invalid jump address"},

		{"valid code and catch", compile.Program{
			Toplevel: &compile.Funcode{
				Code:    []byte{byte(compile.NOP), byte(compile.JMP), '\x06', '\x00', '\x00', '\x00', byte(compile.NOP)},
				Catches: []compile.Defer{{PC0: 1, PC1: 6, StartPC: 0}},
			},
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			p := c.p
			_, err := compile.Dasm(&p)
			if c.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, c.err)
		})
	}
}

func TestAsmRoundtrip(t *testing.T) {
	dir := filepath.Join("..", "..", "starlark", "testdata")
	des, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, de := range des {
		if de.IsDir() || !de.Type().IsRegular() || filepath.Ext(de.Name()) != ".star" {
			continue
		}
		filename := filepath.Join(dir, de.Name())
		for i, chunk := range chunkedfile.Read(filename, t) {
			t.Run(fmt.Sprintf("%s chunk %d", filename, i), func(t *testing.T) {
				predeclared := starlark.StringDict{
					"hasfields": starlark.True,
					"fibonacci": starlark.True,
					"struct":    starlark.NewBuiltin("struct", starlarkstruct.Make),
				}

				opts := getOptions(chunk.Source)
				_, prog, err := starlark.SourceProgramOptions(opts, filename+"_chunk_"+strconv.Itoa(i), chunk.Source, predeclared.Has)
				require.NoError(t, err)

				var buf bytes.Buffer
				require.NoError(t, prog.Write(&buf))
				rawProg, err := compile.DecodeProgram(buf.Bytes())
				require.NoError(t, err)
				clearPosInfo(rawProg)

				// assemble and disassemble prog, should be equivalent to rawProg
				// without position information
				asmData, err := compile.Dasm(rawProg)
				require.NoError(t, err)
				t.Log(string(asmData))
				dasmProg, err := compile.Asm(asmData)
				require.NoError(t, err)

				require.Equal(t, rawProg, dasmProg)
			})
		}
	}
}

func clearPosInfo(p *compile.Program) {
	if len(p.Loads) == 0 {
		p.Loads = nil
	}
	if len(p.Names) == 0 {
		p.Names = nil
	}
	if len(p.Constants) == 0 {
		p.Constants = nil
	}
	if len(p.Functions) == 0 {
		p.Functions = nil
	}
	if len(p.Globals) == 0 {
		p.Globals = nil
	}

	for i, l := range p.Loads {
		l.Pos = syntax.Position{}
		p.Loads[i] = l
	}
	for i, g := range p.Globals {
		g.Pos = syntax.Position{}
		p.Globals[i] = g
	}

	clearFunc := func(fn *compile.Funcode) {
		fn.Pos = syntax.Position{}
		fn.Doc = ""
		fn.ClearPCLineTab()

		if len(fn.Code) == 0 {
			fn.Code = nil
		}
		if len(fn.Locals) == 0 {
			fn.Locals = nil
		}
		if len(fn.Cells) == 0 {
			fn.Cells = nil
		}
		if len(fn.Freevars) == 0 {
			fn.Freevars = nil
		}
		if len(fn.Defers) == 0 {
			fn.Defers = nil
		}
		if len(fn.Catches) == 0 {
			fn.Catches = nil
		}

		for i, l := range fn.Locals {
			l.Pos = syntax.Position{}
			fn.Locals[i] = l
		}
		for i, f := range fn.Freevars {
			f.Pos = syntax.Position{}
			fn.Freevars[i] = f
		}
	}

	clearFunc(p.Toplevel)
	for _, f := range p.Functions {
		clearFunc(f)
	}
}

func getOptions(src string) *syntax.FileOptions {
	return &syntax.FileOptions{
		Set:               option(src, "set"),
		While:             option(src, "while"),
		TopLevelControl:   option(src, "toplevelcontrol"),
		GlobalReassign:    option(src, "globalreassign"),
		LoadBindsGlobally: option(src, "loadbindsglobally"),
		Recursion:         option(src, "recursion"),
	}
}

func option(chunk, name string) bool {
	return strings.Contains(chunk, "option:"+name)
}
