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

#### Scratchpad of ideas

* A new opcode, `DEFERPUSH <addr>`, pushes a `defer` block to the stack (its start address, which is the next pc) and jumps to `addr`, which should be the first instruction after the block.
* Similarly, a new opcode, `CATCHPUSH <addr>`, pushes a `catch` block to the stack and jumps to `addr`.

A big issue is determining efficiently when a block is exited (e.g. on each pc change, check if it is outside the range of the addresses of the block).

See https://cs.stackexchange.com/questions/104886/algorithm-data-structure-to-quickly-check-if-an-integer-is-a-member-of-any-lowe. Worth benchmarking if it does add any noticeable slowdown if there are no intervals to check (no defer/catch), ideally that would not add any overhead in that case.

Or would it be possible to actually use static storage of those blocks, and instead of inserting a call on every exit path, use a single opcode to trigger deferred execution? `RETURN` would be the only one to not require it, it would automatically check if there is deferred execution pending?

Think about it - there are 3 jump operations (`JMP`, `ITERJMP` and `CJMP`) and a single "exit all" one (`RETURN`). All other instructions advance to the next address without jumping (so they can only exit by "fallthrough" outside a block). For the jump opcodes, the deferred execution takes place immediately, _before_ jumping to the destination. For the return opcode, the returned value must be popped and stored temporarily, then execution takes place, and then the return (exit of function) is done. For the fallthrough scenario, the destination address is the current address (i.e. execution must return here after deferred execution).

We could add a single `RUNDEFER` opcode before those 4 operations when there is at least one deferred block pending when jumping to the destination address (or returning). This means looking to more than just the current block's presence of `defer`s or not (i.e. a `RETURN` requires a `RUNDEFER` if there's any in scope for the whole function, while jumps can exit multiple blocks). It sets a flag that is valid for only the next instruction, which must be one of those 4, and indicates that deferred execution procedure is to be applied. It could take an argument indicating if it is a fallthrough (in which case it behaves as if it was followed by a `JMP <pc>`, and deferred execution runs immediately on the `RUNDEFER` opcode, or it takes no argument and is compiled with a subsequent `JMP <next>`). Those opcodes can then pop the deferred blocks and run them as required. This takes care - efficiently - of when to run the deferred blocks. Now we need a way to know when the deferred block exits.

A deferred block can only exit via a fallthrough or a `RETURN` (no explicit jumps outside the block). In particular, it could not do a deferred `break` or `continue` inside a loop, nor a `goto` outside itself. If it contains a `RETURN`, it should be preceded by a `RUNDEFER` if there are pending deferred blocks, just like in non-deferred blocks. Because there can be only one returned value, a `RETURN` in a `defer` overrides any pending `RETURN`, so there's no need to keep a stack of those, only a single state value is required. Return addresses (where to go to after executing deferred blocks), on the other hand, need a stack, because executing a defer can cause execution of other (nested) defers, inside the `defer`, which would also require a destination address to be saved.

For the fallthrough behaviour, a `DEFERJMP` instruction indicates to jump to the last saved destination address in the case of a `defer` (or to return if triggered via a `RETURN` - but those could be mingled with jump destination addresses to process before returning - I think it might be best to prevent `RETURN` in a `defer` for now, just allow them in a `catch`? or same problem (`defer` inside `catch`)?), or to jump to the address following its parent scope exit (which is the argument address always encoded with this opcode, and is required for a `catch`). Before doing so, it checks if there are any more deferred blocks to run (there could be many such blocks, the check for this is done using the source+destination address of what triggered deferred execution - any deferred blocks that were covering the source but not the destination must be executed).

[A description of the steps in the implementation.]

## Rationale

A number of different approaches have been considered (and even attempted) but rejected. They are presented in the following sub-sections.

### Static storage of catch and defer blocks

A strictly static approach to this problem cannot be efficient - every jump/return opcode would need to check if it is entering such a block, exiting it, and exiting the parent scope. This means looping over those defer/catch blocks (or indexing them in a mapping of address to block, but even then the lookups need to be done constantly). This is a lot of overhead to add to a number of general opcodes, which would undoubtedly lead to slowdowns in performance (not verified).

### Insert deferred instructions in all exit paths

At compile-time, it could theoretically be possible to copy the instructions of the deferred blocks (for `defer`, not viable for `catch`) in all the places that the parent scope can exit. But in practice this doesn't quite work - e.g. how do you insert instructions _after_ a `RETURN`? Plus, as mentioned, this doesn't address the `catch` blocks behaviour, and it would drastically increase the code segment of compiled programs.

## Open issues (if applicable)

[A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.]
