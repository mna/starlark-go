### result: 2

program:
	globals:
		result
	constants:
		int 1        # 0
		string "a"   # 1
		int 2        # 2

# do
# 	catch
# 		return 2
# 	end
#		return fn()
# end
function: top 2 0 0
	catches:
		5 8 1
	code:
		JMP  5     # goto makefunc
		CONSTANT 2 # 2
		SETGLOBAL 0 # result = 2
		NONE
		RETURN      # no need to end with CATCHJMP as it would be unreachable

		MAKETUPLE 0 # no args
		MAKEFUNC 0  # fn
		CALL 0
		RETURN

# return 1 + "a"; throws
function: fn 2 0 0
	code:
		CONSTANT 0 # 1
		CONSTANT 1 # "a"
		PLUS			 # 1 + "a"; throws
		RETURN
