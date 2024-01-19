package starlark

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mna/nenuphar/internal/compile"
	"github.com/stretchr/testify/require"
)

var rxAssertGlobal = regexp.MustCompile(`(?m)^\s*###\s*([a-zA-Z][a-zA-Z0-9_]*):\s*(.+)$`)

func TestExecAsm(t *testing.T) {
	dir := filepath.Join("testdata", "asm")
	des, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, de := range des {
		if de.IsDir() || !de.Type().IsRegular() || filepath.Ext(de.Name()) != ".asm" {
			continue
		}
		t.Run(de.Name(), func(t *testing.T) {
			filename := filepath.Join(dir, de.Name())
			b, err := os.ReadFile(filename)
			require.NoError(t, err)

			cprog, err := compile.Asm(b)
			require.NoError(t, err)

			var predeclared StringDict
			var thread Thread
			prog := &Program{cprog}
			out, err := prog.Init(&thread, predeclared)

			// check expectations in the form of '### fail: <error message>' or '###
			// global_name: <value>' (both can be combined, it may fail but still assert
			// some globals)
			ms := rxAssertGlobal.FindAllStringSubmatch(string(b), -1)
			require.NotNil(t, ms, "no assertion provided")
			var errAsserted bool
			for _, m := range ms {
				want := strings.TrimSpace(m[2])
				switch global := m[1]; global {
				case "fail":
					errAsserted = true
					require.ErrorContains(t, err, want)
				case "nofail":
					errAsserted = true
					require.NoError(t, err)
				default:
					// assert the provided global
					g := out[global]
					require.NotNil(t, g, "global %s does not exist", global)
					if want == "None" {
						require.Equal(t, None, g, "global %s", global)
					} else if n, err := strconv.ParseInt(want, 10, 64); err == nil {
						got, err := AsInt32(g)
						require.NoError(t, err)
						require.Equal(t, n, int64(got), "global %s", global)
					} else {
						require.Failf(t, "unexpected result", "global %s: want %s, got %v (%[2]T)", global, want, g)
					}
				}
			}
			if !errAsserted {
				// default to no error expected
				require.NoError(t, err)
			}
		})
	}
}
