package program

import "fmt"

var testISA = NewISA("testISA")

func initTestISA() {
	testISA.registerNewInst("ADD", instADD)
	testISA.registerNewInst("MOV", instMOV)
	testISA.registerNewInst("MAC", instMAC)
}

/*
Entry =>
{
    MOV, [$0], IMM[0]// initialize
    {
        MAC, ![WEST_PORT], ![NORTH_PORT], [$0] -> [$0]
        MOV, ![NORTH_PORT] -> ![SOUTH_PORT] // route the input from above
        ![WEST_PORT] -> ![EAST_PORT] // route the input from left
    }
    {
        MAC, ![WEST_PORT], ![NORTH_PORT], [$0] -> [$0]
        ![NORTH_PORT] -> ![SOUTH_PORT] // route the input from above
        ![EAST_PORT] -> ![WEST_PORT] //
    }
    MOV,  ![NORTH_PORT] -> ![SOUTH_PORT] // send the result of the PE above it
    MOV, [$0] -> ![SOUTH_PORT] // send its own result
}
*/

func main() {
	initTestISA()

	var program = Program{
		EntryBlocks: []EntryBlock{
			{
				EntryCond: OperandList{},
				CombinedInsts: []CombinedInst{
					{
						Insts: []Inst{
							{
								rawtxt:   "MOV, [$0], IMM[0]",
								behavior: instMOV,
								SrcOperands: OperandList{
									Operands: []Operand{
										{
											DynamicID: []DynID{0},
										},
									},
								},
								DstOperands: OperandList{
									Operands: []Operand{
										{
											DynamicID: []DynID{0},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	fmt.Println(program)
}
