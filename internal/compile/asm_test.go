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
