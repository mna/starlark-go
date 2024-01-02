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

var (
	rxFail   = regexp.MustCompile(`(?m)^\s*###\s*fail:\s*(.+)$`)
	rxResult = regexp.MustCompile(`(?m)^\s*###\s*result:\s*(.+)$`)
)

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

			// check expectations in the form of '### fail: <error message>' or '### result: <value>'
			if ms := rxFail.FindSubmatch(b); ms != nil {
				require.ErrorContains(t, err, strings.TrimSpace(string(ms[1])))
			} else if ms := rxResult.FindSubmatch(b); ms != nil {
				result := out["result"]
				require.NotNil(t, result)
				want := strings.TrimSpace(string(ms[1]))
				if want == "None" {
					require.Equal(t, None, result)
				} else if n, err := strconv.ParseInt(want, 10, 64); err == nil {
					got, err := AsInt32(result)
					require.NoError(t, err)
					require.Equal(t, n, int64(got))
				} else {
					require.Failf(t, "unexpected result", "want %s, got %v (%[2]T)", want, result)
				}
			}
		})
	}
}
