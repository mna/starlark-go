### result: 2

program:
	globals:
		result
	constants:
		int 1        # 0
		int 2        # 1

# defer
# 	result = 2
# end
#	result = 1
function: top 2 0 0
	defers:
		4 8 1
	code:
		JMP  4       # goto constant 0
		CONSTANT  1  # 2
		SETGLOBAL 0  # result = 2
		DEFEREXIT

		CONSTANT 0  # 1
		SETGLOBAL 0 # result = 1
		NONE
		RUNDEFER
		RETURN
