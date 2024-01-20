### fail: unknown binary op: int + string

program:
	globals:
		result
	constants:
		int 1        # 0
		string "a"   # 1
		int 2        # 2

# do
# 	catch
# 		result = 2
#			return
# 	end
#		x = fn()
# end
# return x + 'a'
function: top 2 0 0
	locals:
		x
	catches:
		5 8 1
	code:
		JMP  5     # goto maketuple
		CONSTANT 2 # 2
		SETGLOBAL 0 # result = 2
		NONE
		RETURN      # no need to end with CATCHJMP as it would be unreachable

		MAKETUPLE 0 # no args
		MAKEFUNC 0  # fn
		CALL 0
		SETLOCAL 0  # x = fn()
		LOCAL 0			# 1
		CONSTANT 1  # 'a'
		PLUS
		NONE
		RETURN

# return 1
function: fn 1 0 0
	code:
		CONSTANT 0 # 1
		RETURN
