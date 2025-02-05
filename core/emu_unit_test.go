package core

import (
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
})