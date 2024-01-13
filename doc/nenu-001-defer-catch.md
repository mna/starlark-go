# Proposal: Defer and Catch support

Author(s): Martin Angers

Last updated: 2024-01-11

## Abstract

The planned programming language will have support for `defer` and `catch` blocks, which are blocks of code that are not executed when encountered, but instead when the scope that contains them is exited. The difference between both is that `catch` runs only if an exception was raised, while `defer` runs every time.

## Background

The `defer` block is similar to what exists in some modern programming languages (Swift and Go are the ones I'm most familiar with). Its behaviour is most similar to [Swift's](https://docs.swift.org/swift-book/documentation/the-swift-programming-language/statements#Defer-Statement) in that it runs on scope exit, not on function exit, making it suitable to use inside a loop statement.

The `catch` block is identical in use as the `defer` block - it must appear _before_ the code it "protects", and its execution is deferred to when the scope is exited. The difference is that it runs only if the scope exits due to an exception that occurred in the code that follows the `catch` block. The exception is implicitly caught ("recovered") by the block, but may be thrown again (or a different exception may be thrown by the `catch` block). Although I'm not familiar with the language, this seems most similar to Zig's `defer` and `erredefer` statements (except that Zig prevents `return` statements from a `defer`).

## Proposal

The syntax would look something like this:

```
do
	defer
		# always executed
		print('defer')
	end
	catch
		# executed only if an error is raised
		print('catch')
	end
	if x then
		print('ok')
	else
		print('fail')
		throw 'fail'
	end
end
print('after')
```

If `x` is `true`, this would print `ok defer after`, and if it is `false` it would print `fail catch defer after`.

Much like Swift, the statements in the `defer` and `catch` blocks can’t transfer program control outside of the block, execution implicitly resumes after the parent scope once the block is executed. However, `throw` and `return` statements could be allowed (in case of `defer`, if it was executed via a `return` and it executed a `return` itself, the last `return` would "win").

The high-level proposal is already decided to be part of the language, the proposal is about the implementation and as such, the rationale will be moved after the implementation section.

## Compatibility

There is no compatibility concern until v1.

## Implementation

There are a number of points that deferred execution blocks (common to both `defer` and `catch`) must address:

* Nested blocks of arbitrary levels (either directly, a `defer` inside a `defer`, or indirecty, a function call in a `defer` that contains a `defer`)
* Deferred blocks "apply to" or "cover" a number of instructions (all instructions that _follow_ the block, until the end of the containing block)
* The virtual machine must know that a deferred execution is pending when exiting a block
* The virtual machine must know where to jump to skip the deferred block (until it is time to execute it) and go to the next instruction to execute
* The virtual machine must know where a parent block of a deferred execution ends (either by falling through the next instruction outside the block, by jumping outside the block, by returning or by throwing an exception)
* The virtual machine must know where a deferred block ends, to jump to the next instruction after the end of its parent block or to wherever the last non-deferred instruction was jumping to (unless the deferred execution throws or returns)
* The virtual machine only needs to care about deferred blocks in the current function, not about any parent ones - this is because a function is a scope and if there are no deferred blocks in this scope, then there are no deferred blocks to care about - any ones in parent functions will execute after the return of this call.
* If the deferred block is executed following a raised exception, that exception must be made available to the code in some way. If it was not executed following a raised exception (`defer` blocks only), then accessing that exception must return a `nil` value.
* Once the parent block exits, all deferred blocks in that block become dead (after possible eexecution of course).
* If a deferred block causes an exit from multiple blocks (via a throw or a return), then all deferred blocks in those exited blocks must execute as per the rule.

In addition, those points apply specifically to `catch` statements:

* A `catch` implicitly recovers from an exception, the error must be re-thrown (or a new one thrown) to keep the exception alive.
* The previous point means that the stack of current catch statements will only ever run the last one. Then if running that block throws, the next catch statement will run, etc.

### Compiler and VM details

Additional static-time information to collect during compilation:

* List of `defer` blocks:
	- Start address of the `defer`
	- Start and end addresses of the covered instructions

* List of `catch` blocks:
	- Start address of the `catch`
	- Start and end addresses of the covered instructions

* If there's an efficient way to persist an [interval tree](https://en.m.wikipedia.org/wiki/Interval_tree), build it and persist it in the binary, otherwise build it at decode time.

Additional opcodes:

* `RUNDEFER`, an argument-less opcode that _must_ be followed by `JMP`, `ITERJMP`, `CJMP` or `RETURN`.
	- Should be used only if the next instruction causes execution of a `defer` block, but this is an optimization (i.e. it would not cause the program to fail if there is no `defer` to run).
	- Technically, this means that it must be added if the next instruction is covered by a `defer`, but the target of that instruction is not (and a `RETURN` is never a covered target).
	- Triggers execution of any pending `defer` blocks, recording the destination address (or return value) until all deferred blocks have run.
	- If the block exits naturally (a "fallthrough" to the next instruction that follows the block), a `JMP <pc>` instruction must be added so that the requirement that a `RUNDEFER` is always followed by such an instruction is met.

* `DEFEREXIT`, an argument-less opcode that must be present at every exit point of a `defer` block (not `catch` blocks).
	- It doesn't stop execution of deferred blocks, if any other are due to run (because they cover the `DEFEREXIT` instruction but not the target address as stored in the deferred stack - a to-be-returned value or to-be-raised error is never covered), they are run.
	- In other words, `DEFEREXIT` implies a `RUNDEFER` followed by a jump to the return-to value.
	- Otherwise, if there are no more deferred blocks to run, it pops the latest saved return-to value (which may be a value to return, an error to raise or an address to jump to).

* `CATCHJMP <addr>`, an opcode with an argument similar to `DEFEREXIT` that must be present at every exit point of a `catch` block (not `defer` blocks).
	- It doesn't stop execution of deferred blocks, if any other are due to run (because they cover the `CATCHJMP` instruction but not the target address), they are run. In this case, the `CATCHJMP` address is pushed to the deferred stack.
	- Otherwise, if there are no more deferred blocks to run, it jumps to the address that is the argument of the opcode and must be the first instruction following the block containing the `catch` block (the instruction that follows the last instruction covered by the `catch`).
	- If `addr` is `0`, then it does not jump, but instead it returns from the function with a `nil` value. This is for when the `catch` is in the top-level function scope, there is no subsequent instruction to jump to.
	- Need to think properly about how to handle/unroll the deferred stack.

Additional runtime information to dynamically manage:

* The _deferred stack_, which may hold either of those types:
	- a `Value` (the value to return from the function after deferred blocks execution)
	- the address of an instruction (where to jump to after deferred blocks execution)
	- an `error` that was thrown (the error to return from the function after deferred blocks execution)
* The `RUNDEFER` opcode causes the next instruction to push to that stack if there are deferred blocks to run
	- if the next instruction is a `RETURN`, it pops the to-be-returned value from the operands stack and pushes it to the deferred stack
	- otherwise the target address of the jump is pushed to the deferred stack
* The `CATCHJMP` opcode also pushes to that stack if there are deferred blocks to run
	- the target address of the jump is pushed to the deferred stack
	- as a special case, if that address is `0`, it pushes `nil` as a to-be-returned value to the deferred stack
* When an error is thrown, it gets pushed to that deferred stack
	- TODO: review that logic, not obvious how to get the in-flight error and how to clear it on a catch
	- as it runs deferred execution, the "live" error in-flight is the first error present in the stack (and not overridden by a return value)
	- when it runs a `catch` block, it gets popped out but must still be available to be retrieved.
* On `DEFEREXIT`, if there are no more deferred blocks to run, it pops from the deferred stack and executes the corresponding action (return, jump or throw).

Note that a `RETURN` inside a `catch` block does not need anything special outside of those new opcodes - if the `catch` is covered by a `defer`, there will be a `RUNDEFER` before the `RETURN`, and the rest of the behaviour is standard deferred return logic (push returned value to deferred stack, run deferred blocks, ultimately pop and return the value).

An important thing to note is that a `defer` block covers all instructions until the end of the containing block. This includes any implicit `return nil` for the body of a function with no explicit return value.

The next sub-sections will explore the behaviour of the VM under some scenarios.

#### Simple defer

This source code:

```
fn ()
	defer # 1
		A # 2
	end   # 3
	B     # 4
end
```

Compiles to this bytecode:

```
JMP 4
INST A
DEFEREXIT
INST B
NONE
RUNDEFER
RETURN
```

With the following behaviour:

* The `defer` block is compiled with a `JMP` over it so that it does not run immediately.
* The VM runs instruction B, and since this function does not explicitly return anything, it is compiled with the equivalent of `return nil`.
* Because there is a pending `defer` block, the `return` opcode is preceded by a `RUNDEFER`.
* When `RETURN` is executed, it has the `RUNDEFER` flag set, so:
	- it pops and stores the to-be-returned value from the operands stack
	- it looks for the nearest `defer` block that:
		- covers the current `pc` address (the `RETURN` instruction) and
		- does not cover the destination `pc` address (in the case of a `RETURN`, no `defer` block ever covers the destination).
	- if it finds any, it executes it, otherwise it proceeds with the `RETURN` of the value.
* In this scenario, it finds the `defer` block and jumps to `INST A` and starts executing from this instruction.
* When it encounters `DEFEREXIT`, it:
	- looks for the nearest `defer` block with the same criteria (covers the `DEFEREXIT` address, does not cover the target destination address - in this case a to-be-returned value so no block ever covers it)
	- if it finds any, it executes it
	- in this scenario, there is no other `defer` block so it pops from the stored "deferred action" stack and returns the stored to-be-returned value.

#### Nested defers

This source code:

```
fn ()
	defer      # 1
		defer  # 2
			A  # 3
		end    # 4
		B      # 5
	end        # 6
	C          # 7
end
```

Compiles to this bytecode:

```
JMP 7
	JMP 5
		INST A
		DEFEREXIT
	INST B
	DEFEREXIT
INST C
NONE
RUNDEFER
RETURN
```

With the following behaviour:

* Execution starts with instruction C, jumping over the deferred block.
* Before the fallthrough (implicit) `return nil`, the outermost defer is executed as it covers the `RETURN` instruction.
* Instruction B is executed, and on `DEFEREXIT` it looks for any deferred blocks that covers the `DEFEREXIT` but not the destination (and since the destination is a `RETURN`, no block covers it).
* It finds the nested `defer` and executes instruction A, and repeats the logic on its `DEFEREXIT`, finding no other deferred block, so it returns the pending return value.

#### Multiple defers

This source code:

```
fn ()
	defer  # 1
		A  # 2
	end    # 3
	defer  # 4
		B  # 5
	end    # 6
	C      # 7
end
```

Compiles to this bytecode:

```
JMP 4
	INST A
	DEFEREXIT
JMP 7
	INST B
	DEFEREXIT
INST C
NONE
RUNDEFER
RETURN
```

With the following behaviour:

* As usual, a `defer` block is preceded by a `JMP` over it (to the instruction that follows the `defer` block).
* It starts executing instruction C, then at the `RETURN` flagged with `RUNDEFER` it looks for the nearmost `defer` that covers the `RETURN`, storing the to-be-returned value on the deferred stack.
* It executes instruction B, and on `DEFEREXIT` repeats the process of looking at the nearest `defer` that covers it and finds one.
* It executes instruction A and on `DEFEREXIT` again looks for a `defer` that covers it, doesn't find any, and pops from the deferred stack and returns the value.

#### Simple catch

This source code:

```
fn ()
	catch  # 1
		A  # 2
	end    # 3
	B      # 4
end
```

Compiles to this bytecode:

```
JMP 4
	INST A
	CATCHJMP 0
INST B
NONE
RETURN
```

With the following behaviour:

* As for `defer` blocks, a `JMP` jumps over the `catch` block.
* It starts executing instruction B.
* There is no `RUNDEFER` because there is no pending `defer` block, a `catch` only runs on error so if there's no error with instruction B, execution continues normally with no special behaviour for the `catch`.
* If instruction B throws an error, the VM looks for the nearest `catch` block and jumps to it if it finds one, otherwise it returns the error to the caller.
* If it finds one - as in this scenario -, it runs instruction A and on `CATCHJMP`:
	- if there are any `defer` blocks to run, it runs the nearest one (pushing the target address or return value on the deferred stack), and then processing resumes as if it was a standard deferred execution;
	- otherwise it jumps to the provided address or - as in this case - returns `nil` if it's a function's top-level `catch` statement (i.e. there are no more instructions to jump to after the ones covered by the `catch`).

#### Catch with re-throw and defers

This source code:

```
fn ()
	defer 			# 1
		A           # 2
	end             # 3
	catch           # 4
		B           # 5
		rethrow     # 6
	end             # 7
	defer           # 8
		C           # 9
	end             # 10
	D               # 11
end
```

Compiles to this bytecode:

```
JMP 4
	INST A
	DEFEREXIT
JMP 8
	INST B
	THROW
JMP 11
	INST C
	DEFEREXIT
INST D
NONE
RUNDEFER
RETURN
```

With the following behaviour:

* Execution starts with instruction D, as per the jumps over deferred blocks.
* If instruction D succeeds, the `RETURN` flagged with `RUNDEFER` pushes the return value to the deferred stack and causes execution of instruction C, then `DEFEREXIT` causes execution of instruction A, and finally its `DEFEREXIT` pops the return value and returns it.
* If instruction D fails, it pushes the error on the deferred stack and runs instruction C (nearest defer block), then on `DEFEREXIT` it looks for other deferred blocks and runs the `catch` block - instruction B - which pops the error from the stack (it is handled), but the re-throw re-pushes it to the stack and the nearest `defer` block is identified and instruction A is run, and on its `DEFEREXIT` the error is popped from the stack (no more deferred block to run) and is returned.
* If instead the `catch` block had not re-thrown, it would've ended with `CATCHJMP` and the address would've been pushed to the deferred stack, and then the top-most `defer` block with instruction A would've run and on `DEFEREXIT` would've resumed at the catch-jump address (which in this case would've been `0` so `return nil`).

#### Nested catches with re-throw

#### Multiple catches with re-throw

## Rationale

A number of different approaches have been considered (and even attempted) but rejected. They are presented in the following sub-sections.

### Static storage of catch and defer blocks

A strictly static approach (without any new opcode) to this problem cannot be efficient - every jump/return opcode would need to check if it is entering such a block, exiting it, and exiting the parent scope. This means looping over those defer/catch blocks (or indexing them in a mapping of address to block, but even then the lookups need to be done constantly). This is a lot of overhead to add to a number of general opcodes, which would undoubtedly lead to slowdowns in performance (not verified).

### Insert deferred instructions in all exit paths

At compile-time, it could theoretically be possible to copy the instructions of the deferred blocks (for `defer`, not viable for `catch`) in all the places that the parent scope can exit. But in practice this doesn't quite work - e.g. how do you insert instructions _after_ a `RETURN`? Plus, as mentioned, this doesn't address the `catch` blocks behaviour, and it would drastically increase the code segment of compiled programs.

## Open issues (if applicable)

[A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.]
