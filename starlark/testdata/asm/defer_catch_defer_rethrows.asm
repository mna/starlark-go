### result: "?ad"
### fail: unknown binary op: string + int

program:
	globals:
		result
	constants:
		string "a"        # 0
		string "b"        # 1
		string "c"        # 2
		string "d"        # 3
		string "?"        # 4
		int 1             # 5

# defer
# 	result = result + 'd'
# end
# catch
#   result = result + 1
# end
# defer
#   result = result + 'a'
# end
#	result = '?'
# result = result + 1
function: top 2 0 0
	defers:
		6 26 1
		18 26 13
	catches:
		12 26 7
	code:
		JMP  6
		GLOBAL 0     # result
		CONSTANT  3  # 'd'
		PLUS
		SETGLOBAL 0  # result = result + 'd'
		DEFEREXIT

		JMP  12
		GLOBAL 0		 # result
		CONSTANT  5  # 1
		PLUS
		SETGLOBAL 0  # result = result + 1
		CATCHJMP 0

		JMP  18
		GLOBAL 0		 # result
		CONSTANT  0  # 'a'
		PLUS
		SETGLOBAL 0  # result = result + 'a'
		DEFEREXIT

		CONSTANT 4  # '?'
		SETGLOBAL 0 # result = '?'
		GLOBAL 0    # result
		CONSTANT 5  # 1
		PLUS
		SETGLOBAL 0  # result = result + 1, throws
		NONE
		RUNDEFER
		RETURN
