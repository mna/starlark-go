# Proposal: Defer and Catch support

Author(s): Martin Angers

Last updated: 2024-01-04

## Abstract

The planned programming language will have support for `defer` and `catch` blocks, which are blocks of code that are not executed when encountered by the virtual machine, but instead when the scope that contains them is exited. The difference between both is that `catch` runs only if an exception was raised, while `defer` runs every time.

## Background

The `defer` block is similar to what exists in some modern programming languages (Swift and Go are the ones I'm most familiar with). Its behaviour is most similar to [Swift's](https://docs.swift.org/swift-book/documentation/the-swift-programming-language/statements#Defer-Statement) in that it runs on scope exit, not on function exit, making it suitable to use inside a loop statement.

The `catch` block is identical in use as the `defer` block - it must appear _before_ the code it "protects", and its execution is deferred to when the scope is exited. The difference is that it runs only if the scope exits due to an exception that occurred in the code that follows the `catch` block. The exception is implicitly caught ("recovered") by the block, but may be thrown again (or a different exception may be thrown by the `catch` block).

## Proposal

The syntax would look something like this:

```
do
	defer
		print('defer')
	end
	catch
		print('catch')
	end
	if x then
		print('ok')
	else
		print('fail')
		throw 'fail'
	end
end
```

Much like Swift, the statements in the `defer` and `catch` blocks can’t transfer program control outside of the block, execution implicitly resumes after the parent scope once the block is executed. However, `throw` and `return` statements could be allowed (in case of `defer`, if it was executed via a `return` and it executed a `return` itself, the last `return` would "win").

The high-level proposal is already decided to be part of the language, the proposal is about the implementation and as such, the rationale will be moved after the implementation section.

## Compatibility

There is no compatibility concern until v1.

## Implementation

There are a number of points that deferred execution blocks (common to both `defer` and `catch`) must address:

* Nested blocks of arbitrary levels (either directly, a `defer` inside a `defer`, or indirecty, a function call in a `defer` that contains a `defer`)
* Deferred blocks "apply to" or "cover" a number of instructions (all instructions that _follow_ the block, until the end of the containing block)
* The virtual machine must be able to know that a deferred execution is pending when exiting a block
* The virtual machine must be able to know where to jump to skip the deferred block (until it is time to execute it) and go to the next instruction to execute
* The virtual machine must be able to know where a parent block of a deferred execution ends (either by falling through the next instruction outside the block, by jumping outside the block, by returning or by throwing an exception)
* The virtual machine must be able to know where a deferred block ends, to jump to the next instruction after the end of its parent block (unless the deferred execution throws or returns)
* The virtual machine only needs to care about deferred blocks in the current function, not about any parent ones - this is because a function is a scope and if there are no deferred blocks in this scope, then there are no deffered blocks to care about - any ones in parent functions will execute after the return of this call.
* If the deferred block is executed following a raised exception, that exception must be made available to the code in some way. If it was not executed following a raised exception (`defer` blocks only), then accessing that exception must return a nil value.

In addition, those points apply specifically to `catch` statements:

[A description of the steps in the implementation.]

## Rationale

[A discussion of alternate approaches and the trade offs, advantages, and disadvantages of the specified approach.]

## Open issues (if applicable)

[A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none.]
