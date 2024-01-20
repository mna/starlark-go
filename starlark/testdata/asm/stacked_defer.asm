### x: 1
### y: 2
### z: 3

program:
	globals:
		x
		y
		z
	constants:
		int 0        # 0
		int 1        # 1

# defer
#   i = i + 1
# 	z = i
# end
# defer
#   i = i + 1
# 	y = i
# end
# defer
#   i = i + 1
# 	x = i
# end
#	i = 0
function: top 2 0 0
	locals:
		i
	defers:
		8 28 1
		16 28 9
		24 28 17
	code:
		JMP  8       # goto next defer
		CONSTANT  1  # 1
		LOCAL 0      # i
		PLUS
		SETLOCAL 0   # i = i + 1
		LOCAL 0      # i
		SETGLOBAL 2  # z = i
		DEFEREXIT

		JMP  16      # goto next defer
		CONSTANT  1  # 1
		LOCAL 0      # i
		PLUS
		SETLOCAL 0   # i = i + 1
		LOCAL 0      # i
		SETGLOBAL 1  # y = i
		DEFEREXIT

		JMP  24      # goto main
		CONSTANT  1  # 1
		LOCAL 0      # i
		PLUS
		SETLOCAL 0   # i = i + 1
		LOCAL 0      # i
		SETGLOBAL 0  # x = i
		DEFEREXIT

		CONSTANT 0  # 0
		SETLOCAL 0  # i = 0
		NONE
		RUNDEFER
		RETURN
