package starlark

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mna/nenuphar/internal/compile"
	"github.com/stretchr/testify/require"
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
			require.NoError(t, err)
			t.Log(out)
		})
	}
}
