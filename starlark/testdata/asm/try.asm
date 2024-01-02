program:
	globals:
		result
	constants:
		int 1        # 0
		string "a"   # 1

function: top 2 0 0  # x = try 1 + "a"
	locals:
		x  					# 0
	catches:
		3 5 1
	code:
		JMP  3     # goto try
		NONE
		JMP  6     # goto setlocal

		# try 1 + "a"
		CONSTANT 0 # 1
		CONSTANT 1 # "a"
		PLUS			 # 1 + "a"; throws

		# x = result
		SETLOCAL 0

		LOCAL 0
		SETGLOBAL 0 # result = x
		NONE
		RETURN
