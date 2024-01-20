### result: "?acd"

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
#   result = result + 'c'
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
		6 32 1
		24 32 19
	catches:
		12 32 7
		18 32 13
	code:
		JMP  6
		GLOBAL 0     # result
		CONSTANT  3  # 'd'
		PLUS
		SETGLOBAL 0  # result = result + 'd'
		DEFEREXIT

		JMP  12
		GLOBAL 0		 # result
		CONSTANT  2  # 'c'
		PLUS
		SETGLOBAL 0  # result = result + 'c'
		CATCHJMP 0

		JMP  18
		GLOBAL 0		 # result
		CONSTANT  5  # 1
		PLUS
		SETGLOBAL 0  # result = result + 1
		CATCHJMP 0

		JMP  24
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
