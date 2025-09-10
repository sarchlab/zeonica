Core 0,0:
PHI_CONST 0, East -> East, South, $0
ADD 0, $0 -> $1
LOAD $1 -> South


Core 0,1:
MOV 114 -> West
ADD 1, West -> South, $0
GPRED $0, South -> West
JMP 1

Core 1,0:
ADD 2, North -> $0
LDD $0 -> $1
MUL $1, North -> South

Core 1,1:
LT_EX North, 2 -> North

Core 2,0:
MOV 514 -> East
ADD East, North -> $0, East
STD 0, $0
JMP 1

Core 2,1:
PHI_CONST 2, West -> West