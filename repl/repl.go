// Package repl provides a read/eval/print loop for Starlark.
//
// It supports readline-style command editing,
// and interrupts through Control-C.
//
// If an input line can be parsed as an expression,
// the REPL parses and evaluates it and prints its result.
// Otherwise the REPL reads lines until a blank line,
// then tries again to parse the multi-line input as an
// expression. If the input still cannot be parsed as an expression,
// the REPL parses and executes it as a file (a list of statements),
// for side effects.
package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/chzyer/readline"
	"github.com/mna/nenuphar/starlark"
	"github.com/mna/nenuphar/syntax"
)

var interrupted = make(chan os.Signal, 1)

// REPL executes a read, eval, print loop.
//
// Before evaluating each expression, it sets the Starlark thread local
// variable named "context" to a context.Context that is cancelled by a
// SIGINT (Control-C). Client-supplied global functions may use this
// context to make long-running operations interruptable.
func REPL(opts *syntax.FileOptions, thread *starlark.Thread, globals starlark.StringDict) {
	signal.Notify(interrupted, os.Interrupt)
	defer signal.Stop(interrupted)

	rl, err := readline.New(">>> ")
	if err != nil {
		PrintError(err)
		return
	}
	defer rl.Close()
	for {
		if err := rep(opts, rl, thread, globals); err != nil {
			if err == readline.ErrInterrupt {
				fmt.Println(err)
				continue
			}
			break
		}
	}
}

// rep reads, evaluates, and prints one item.
//
// It returns an error (possibly readline.ErrInterrupt)
// only if readline failed. Starlark errors are printed.
func rep(opts *syntax.FileOptions, rl *readline.Instance, thread *starlark.Thread, globals starlark.StringDict) error {
	// Each item gets its own context,
	// which is cancelled by a SIGINT.
	//
	// Note: during Readline calls, Control-C causes Readline to return
	// ErrInterrupt but does not generate a SIGINT.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-interrupted:
			cancel()
		case <-ctx.Done():
		}
	}()

	thread.SetLocal("context", ctx)

	eof := false

	// readline returns EOF, ErrInterrupted, or a line including "\n".
	rl.SetPrompt(">>> ")
	readline := func() ([]byte, error) {
		line, err := rl.Readline()
		rl.SetPrompt("... ")
		if err != nil {
			if err == io.EOF {
				eof = true
			}
			return nil, err
		}
		return []byte(line + "\n"), nil
	}

	// Treat load bindings as global (like they used to be) in the REPL.
	// Fixes github.com/google/starlark-go/issues/224.
	opts2 := *opts
	opts2.LoadBindsGlobally = true
	opts = &opts2

	// parse
	f, err := opts.ParseCompoundStmt("<stdin>", readline)
	if err != nil {
		if eof {
			return io.EOF
		}
		PrintError(err)
		return nil
	}

	if expr := soleExpr(f); expr != nil {
		// eval
		v, err := starlark.EvalExprOptions(f.Options, thread, expr, globals)
		if err != nil {
			PrintError(err)
			return nil
		}

		// print
		if v != starlark.None {
			fmt.Println(v)
		}
	} else if err := starlark.ExecREPLChunk(f, thread, globals); err != nil {
		PrintError(err)
		return nil
	}

	return nil
}

func soleExpr(f *syntax.File) syntax.Expr {
	if len(f.Stmts) == 1 {
		if stmt, ok := f.Stmts[0].(*syntax.ExprStmt); ok {
			return stmt.X
		}
	}
	return nil
}

// PrintError prints the error to stderr,
// or its backtrace if it is a Starlark evaluation error.
func PrintError(err error) {
	if evalErr, ok := err.(*starlark.EvalError); ok {
		fmt.Fprintln(os.Stderr, evalErr.Backtrace())
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
}

// MakeLoad calls [MakeLoadOptions] using [syntax.LegacyFileOptions].
// Deprecated: relies on legacy global variables.
func MakeLoad() func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	return MakeLoadOptions(syntax.LegacyFileOptions())
}

// MakeLoadOptions returns a simple sequential implementation of module loading
// suitable for use in the REPL.
// Each function returned by MakeLoadOptions accesses a distinct private cache.
func MakeLoadOptions(opts *syntax.FileOptions) func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	type entry struct {
		globals starlark.StringDict
		err     error
	}

	var cache = make(map[string]*entry)

	return func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
		e, ok := cache[module]
		if e == nil {
			if ok {
				// request for package whose loading is in progress
				return nil, fmt.Errorf("cycle in load graph")
			}

			// Add a placeholder to indicate "load in progress".
			cache[module] = nil

			// Load it.
			thread := &starlark.Thread{Name: "exec " + module, Load: thread.Load}
			globals, err := starlark.ExecFileOptions(opts, thread, module, nil, nil)
			e = &entry{globals, err}

			// Update the cache.
			cache[module] = e
		}
		return e.globals, e.err
	}
}
