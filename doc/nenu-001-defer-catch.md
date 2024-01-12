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

* If there's an efficient way to persist an interval tree? If so, build it and persist it in the binary, otherwise build it at decode time.

Additional opcodes:

* `RUNDEFER`, an opcode that _must_ be followed by `JMP`, `ITERJMP`, `CJMP` or `RETURN`.
	- Should be used only if the next instruction causes execution of a `defer` block, but this is an optimization (i.e. it would not cause the program to fail if there is no `defer` to run).
	- Triggers execution of any pending `defer` blocks, recording the destination address (or return value) until all deferred blocks have run.
	- If the block exits naturally (a "fallthrough" to the next instruction that follows the block), a `JMP <pc>` instruction must be added so that the requirement that a `RUNDEFER` is always followed by such an instruction is met.

* `DEFEREXIT`, an opcode that must be present at every exit point of a `defer` block (not `catch` blocks).
	- It doesn't stop execution of deferred blocks, if any other are due to run, they are run.
	- Otherwise, if there are no more deferred blocks to run, it pops the latest saved return-to value (which may be a value to return or an address to jump to).

* `CATCHJMP <addr>`, an opcode similar to `DEFERJMP` that must be present at every exit point of a `catch` block (not `defer` blocks).
	- It doesn't stop execution of deferred blocks, if any other are due to run, they are run.
	- Otherwise, if there are no more deferred blocks to run, it jumps to the address that is the argument of the opcode and must be the first instruction following the block containing the `catch` block (the instruction that follows the last instruction covered by the `catch`).

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
	- it pops and stores the to-be-returned value
	- it looks for the nearest `defer` block that:
		- covers the current `pc` address (the `RETURN` instruction) and
		- does not cover the destination `pc` address (in the case of a `RETURN`, no `defer` block ever covers the destination).
	- if it finds any, it executes it, otherwise it proceeds with the `RETURN` of the value.
* In this scenario, it finds the `defer` block and jumps to `INST A` and starts executing from this instruction.
* When it encounters `DEFEREXIT`, it:
	- looks for the nearest `defer` block with the same criteria (covers the `DEFEREXIT` address, does not cover the target destination address - in this case a to-be-returned value so no block ever covers it)
	- if it finds any, it executes it
	- in this scenario, there is no other `defer` block so it pops from the stored "deferred action" stack and returns the stored to-be-returned value.

#### Nested defer

#### Simple catch

#### Catch with defer

#### Nested catch with re-throw

## Rationale

A number of different approaches have been considered (and even attempted) but rejected. They are presented in the following sub-sections.

### Static storage of catch and defer blocks

A strictly static approach (without any new opcode) to this problem cannot be efficient - every jump/return opcode would need to check if it is entering such a block, exiting it, and exiting the parent scope. This means looping over those defer/catch blocks (or indexing them in a mapping of address to block, but even then the lookups need to be done constantly). This is a lot of overhead to add to a number of general opcodes, which would undoubtedly lead to slowdowns in performance (not verified).

### Insert deferred instructions in all exit paths

At compile-time, it could theoretically be possible to copy the instructions of the deferred blocks (for `defer`, not viable for `catch`) in all the places that the parent scope can exit. But in practice this doesn't quite work - e.g. how do you insert instructions _after_ a `RETURN`? Plus, as mentioned, this doesn't address the `catch` blocks behaviour, and it would drastically increase the code segment of compiled programs.

## Open issues (if applicable)

[A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.]
