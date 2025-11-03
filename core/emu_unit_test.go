package core

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"math"
)

var _ = Describe("InstEmulator", func() {
	var (
		ie instEmulator
		s  coreState
	)

	BeforeEach(func() {
		ie = instEmulator{}
		s = coreState{
			PC:        		  0,
			TileX:     		  0,
			TileY:     		  0,
			Registers: 		  make([]uint32, 16),
			Memory:    		  make([]uint32, 1024),
			Code:             make([]string, 0),
			RecvBufHead:      make([][]uint32, 4),
			RecvBufHeadReady: make([][]bool, 4),
			SendBufHead:      make([][]uint32, 4),
			SendBufHeadBusy:  make([][]bool, 4),
		}
	})

	Context("Arithmetic Instructions", func() {
		Describe("MUL_CONST", func() {
			It("should multiply register by immediate", func() {
				s.Registers[0] = 5
				ie.RunInst("MUL_CONST, $1, $0, 3", &s)
				Expect(s.Registers[1]).To(Equal(uint32(15)))
				Expect(s.PC).To(Equal(uint32(1)))
			})

			// It("should handle negative values", func() {
			// 	s.Registers[0] = uint32(int32(-5))
			// 	ie.RunInst("MUL_CONST, $1, $0, 3", &s)
			// 	Expect(int32(s.Registers[1])).To(Equal(int32(-15)))
			// })
		})

		Describe("MUL_CONST_ADD", func() {
			It("should multiply and accumulate", func() {
				s.Registers[0] = 5
				s.Registers[1] = 10
				ie.RunInst("MUL_CONST_ADD, $1, $0, 3", &s)
				Expect(s.Registers[1]).To(Equal(uint32(10 + 5*3)))
			})
		})

		Describe("MUL_SUB", func() {
			It("should multiply and subtract", func() {
				s.Registers[0] = 5
				s.Registers[1] = 20
				s.Registers[2] = 2
				ie.RunInst("MUL_SUB, $1, $0, $2", &s)
				Expect(s.Registers[1]).To(Equal(uint32(20 - 2*5)))
			})
		})

		Describe("DIV", func() {
			It("should perform integer division", func() {
				s.Registers[0] = 15
				s.Registers[1] = 4
				ie.RunInst("DIV, $2, $0, $1", &s)
				Expect(s.Registers[2]).To(Equal(uint32(3)))
			})

			It("should panic on division by zero", func() {
				s.Registers[0] = 5
				s.Registers[1] = 0
				Expect(func() {
					ie.RunInst("DIV, $2, $0, $1", &s)
				}).To(Panic())
			})
		})
	})

	Context("Bitwise Instructions", func() {
		Describe("LLS", func() {
			It("should perform logical left shift", func() {
				s.Registers[0] = 0x0000000F // 15
				ie.RunInst("LLS, $1, $0, 4", &s)
				Expect(s.Registers[1]).To(Equal(uint32(0x000000F0)))
			})

			It("should handle overflow", func() {
				s.Registers[0] = 0x80000000
				ie.RunInst("LLS, $1, $0, 1", &s)
				Expect(s.Registers[1]).To(Equal(uint32(0x00000000)))
			})
		})

		Describe("LRS", func() {
			It("should perform logical right shift", func() {
				s.Registers[0] = 0xF0000000
				ie.RunInst("LRS, $1, $0, 4", &s)
				Expect(s.Registers[1]).To(Equal(uint32(0x0F000000)))
			})
		})

		Describe("OR/XOR/AND/NOT", func() {
			BeforeEach(func() {
				s.Registers[0] = 0x0F0F0F0F
				s.Registers[1] = 0x00FF00FF
			})

			It("OR operation", func() {
				ie.RunInst("OR, $2, $0, $1", &s)
				Expect(s.Registers[2]).To(Equal(uint32(0x0FFF0FFF)))
			})

			It("XOR operation", func() {
				ie.RunInst("XOR, $2, $0, $1", &s)
				Expect(s.Registers[2]).To(Equal(uint32(0x0FF00FF0)))
			})

			It("AND operation", func() {
				ie.RunInst("AND, $2, $0, $1", &s)
				Expect(s.Registers[2]).To(Equal(uint32(0x000F000F)))
			})

			It("NOT operation", func() {
				ie.RunInst("NOT, $2, $0", &s)
				Expect(s.Registers[2]).To(Equal(uint32(0xF0F0F0F0)))
			})
		})
	})

	Context("Memory Instructions", func() {
		Describe("LD/ST", func() {
			It("should store/load from immediate address", func() {
				s.Registers[0] = 0xDEADBEEF
				ie.RunInst("ST, $0, 0x100", &s) // Store
				ie.RunInst("LD, $1, 0x100", &s)  // Load
				Expect(s.Registers[1]).To(Equal(uint32(0xDEADBEEF)))
			})

			It("should handle register-based addressing", func() {
				s.Registers[0] = 0x100
				s.Registers[1] = 0xCAFEBABE
				ie.RunInst("ST, $1, $0", &s)       // Store at 0x100
				ie.RunInst("LD, $2, $0", &s)       // Load from 0x100
				Expect(s.Registers[2]).To(Equal(uint32(0xCAFEBABE)))
			})

			It("should panic on out-of-bounds access", func() {
				Expect(func() {
					ie.RunInst("LD, $0, 0xFFFFFFFF", &s)
				}).To(Panic())
			})
		})
	})

	Context("Floating Point Instructions", func() {
		Describe("FADD/FSUB/FMUL/FDIV/FINC/FMUL_CONST/FADD_CONST", func() {
			It("should perform Floating add operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				s.Registers[1] = math.Float32bits(2.71)
				ie.RunInst("FADD, $2, $0, $1", &s)
				result := math.Float32frombits(s.Registers[2])
				Expect(result).To(BeNumerically("~", 5.85, 1e-6))
			})

			It("should perform Floating subtraction operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				s.Registers[1] = math.Float32bits(2.71)
				ie.RunInst("FSUB, $2, $0, $1", &s)
				result := math.Float32frombits(s.Registers[2])
				Expect(result).To(BeNumerically("~", 0.43, 1e-6))
			})

			It("should perform Floating multiplication operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				s.Registers[1] = math.Float32bits(2.71)
				ie.RunInst("FMUL, $2, $0, $1", &s)
				result := math.Float32frombits(s.Registers[2])
				Expect(result).To(BeNumerically("~", 3.14*2.71, 1e-6))
			})

			It("should perform Floating division operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				s.Registers[1] = math.Float32bits(2.71)
				ie.RunInst("FDIV, $2, $0, $1", &s)
				result := math.Float32frombits(s.Registers[2])
				Expect(result).To(BeNumerically("~", 3.14/2.71, 1e-6))
			})

			It("should perform Floating increment operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				ie.RunInst("FINC, $0", &s)
				result := math.Float32frombits(s.Registers[0])
				Expect(result).To(BeNumerically("~", 4.14, 1e-6))
			})

			It("should perform Floating add const operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				ie.RunInst("FADD_CONST, $2, $0, 1.78", &s)
				result := math.Float32frombits(s.Registers[2])
				Expect(result).To(BeNumerically("~", 3.14+1.78, 1e-4))
			})
			
			It("should perform Floating multiply const operation", func() {
				s.Registers[0] = math.Float32bits(3.14)
				ie.RunInst("FMUL_CONST, $2, $0, 1.8", &s)
				result := math.Float32frombits(s.Registers[2])
				Expect(result).To(BeNumerically("~", 3.14*1.8, 1e-4))
			})


			It("should handle special values", func() {
				// Test Infinity
				s.Registers[0] = math.Float32bits(float32(math.Inf(1)))
				s.Registers[1] = math.Float32bits(3.14)
				ie.RunInst("FADD, $2, $0, $1", &s)
				Expect(math.IsInf(float64(math.Float32frombits(s.Registers[2])), 1)).To(BeTrue())

				// Test NaN
				s.Registers[0] = math.Float32bits(float32(math.NaN()))
				ie.RunInst("FADD, $2, $0, $1", &s)
				Expect(math.IsNaN(float64(math.Float32frombits(s.Registers[2])))).To(BeTrue())
			})
		})
	})

	Context("PHI Instruction with IR", func() {
		Describe("PHI based on predecessor block", func() {
			It("should select value from correct predecessor block", func() {
				// Build a simple program with PHI instruction
				// A:
				//   MOV, $1, 100
				//   JMP, B
				// C:
				//   MOV, $2, 200
				//   JMP, B
				// B:
				//   PHI, $0, $1@A, $2@C
				program := []string{
					"A:",
					"MOV, $1, 100",
					"JMP, B",
					"C:",
					"MOV, $2, 200",
					"JMP, B",
					"B:",
					"PHI, $0, $1@A, $2@C",
					"DONE",
				}

				// Initialize ProgramIR and PCToBlock
				s.Code = program
				s.ProgramIR = make([]Instruction, len(program))
				s.PCToBlock = make(map[uint32]string)
				
				currentBlock := ""
				for i, line := range program {
					line = strings.TrimSpace(line)
					if strings.HasSuffix(line, ":") {
						labelName := strings.TrimSuffix(line, ":")
						labelName = strings.TrimSpace(labelName)
						s.ProgramIR[i] = Instruction{
							Label: labelName,
							Raw:   line,
						}
						currentBlock = labelName
					} else {
						tokens := strings.Split(line, ",")
						opcode := ""
						operands := []string{}
						if len(tokens) > 0 {
							opcode = strings.TrimSpace(tokens[0])
							for j := 1; j < len(tokens); j++ {
								operands = append(operands, strings.TrimSpace(tokens[j]))
							}
						}
						s.ProgramIR[i] = Instruction{
							Opcode:   Opcode(opcode),
							Operands: operands,
							Raw:      line,
						}
						if currentBlock != "" {
							s.PCToBlock[uint32(i)] = currentBlock
						}
					}
				}

				// Simulate coming from block A
				s.PC = 0
				s.CurrentBlock = ""
				s.LastPredBlock = ""

				// Execute A: label (PC=0)
				ir := s.ProgramIR[0]
				Expect(ir.Label).To(Equal("A"))
				s.PC++

				// Execute MOV $1, 100 (PC=1)
				s.CurrentBlock = "A"
				ie.RunInst("MOV, $1, 100", &s)
				Expect(s.Registers[1]).To(Equal(uint32(100)))
				Expect(s.PC).To(Equal(uint32(2)))

				// Execute JMP B (PC=2) - this sets PC to B's label
				ie.RunInst("JMP, B", &s)
				Expect(s.PC).To(Equal(uint32(7))) // PC should jump to "B:" label + 1

				// Now we're entering block B from A
				// Update LastPredBlock before executing PHI
				s.LastPredBlock = "A"
				s.CurrentBlock = "B"

				// Execute PHI (PC=7)
				phiInst := s.ProgramIR[7]
				Expect(string(phiInst.Opcode)).To(Equal("PHI"))
				ie.RunInstIR(phiInst, &s)
				
				// Should have selected value from $1 because predecessor was A
				Expect(s.Registers[0]).To(Equal(uint32(100)))
				Expect(s.PC).To(Equal(uint32(8)))
			})

			It("should select value from alternate predecessor block", func() {
				// Same program but coming from C instead of A
				program := []string{
					"A:",
					"MOV, $1, 100",
					"JMP, B",
					"C:",
					"MOV, $2, 200",
					"JMP, B",
					"B:",
					"PHI, $0, $1@A, $2@C",
					"DONE",
				}

				s.Code = program
				s.ProgramIR = make([]Instruction, len(program))
				s.PCToBlock = make(map[uint32]string)
				
				currentBlock := ""
				for i, line := range program {
					line = strings.TrimSpace(line)
					if strings.HasSuffix(line, ":") {
						labelName := strings.TrimSuffix(line, ":")
						labelName = strings.TrimSpace(labelName)
						s.ProgramIR[i] = Instruction{
							Label: labelName,
							Raw:   line,
						}
						currentBlock = labelName
					} else {
						tokens := strings.Split(line, ",")
						opcode := ""
						operands := []string{}
						if len(tokens) > 0 {
							opcode = strings.TrimSpace(tokens[0])
							for j := 1; j < len(tokens); j++ {
								operands = append(operands, strings.TrimSpace(tokens[j]))
							}
						}
						s.ProgramIR[i] = Instruction{
							Opcode:   Opcode(opcode),
							Operands: operands,
							Raw:      line,
						}
						if currentBlock != "" {
							s.PCToBlock[uint32(i)] = currentBlock
						}
					}
				}

				// Simulate coming from block C
				s.PC = 3 // Start at C:
				s.CurrentBlock = ""
				s.LastPredBlock = ""
				s.PC++ // Skip C: label

				// Execute MOV $2, 200 (PC=4)
				s.CurrentBlock = "C"
				ie.RunInst("MOV, $2, 200", &s)
				Expect(s.Registers[2]).To(Equal(uint32(200)))

				// Execute JMP B (PC=5)
				ie.RunInst("JMP, B", &s)
				Expect(s.PC).To(Equal(uint32(7)))

				// Now we're entering block B from C
				s.LastPredBlock = "C"
				s.CurrentBlock = "B"

				// Execute PHI
				phiInst := s.ProgramIR[7]
				ie.RunInstIR(phiInst, &s)
				
				// Should have selected value from $2 because predecessor was C
				Expect(s.Registers[0]).To(Equal(uint32(200)))
			})
		})
	})
})