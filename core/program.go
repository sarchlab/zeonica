package core

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CoreProgram represents a program for a specific core
type CoreProgram struct {
	Row     int         `yaml:"row"`
	Column  int         `yaml:"column"`
	CoreID  string      `yaml:"core_id"`
	Entries []YAMLEntry `yaml:"entries"`
}

// YAMLEntry represents an entry block in the YAML
type YAMLEntry struct {
	EntryID      string        `yaml:"entry_id"`
	Type         string        `yaml:"type"`
	Instructions []Instruction `yaml:"instructions"`
}

// Instruction represents a single instruction in the YAML
type Instruction struct {
	OpCode      string        `yaml:"opcode"`
	SrcOperands []YAMLOperand `yaml:"src_operands"`
	DstOperands []YAMLOperand `yaml:"dst_operands"`
}

// YAMLOperand represents an operand in the YAML
type YAMLOperand struct {
	Operand string `yaml:"operand"`
	Color   string `yaml:"color"`
}

// ArrayConfig represents the top-level YAML structure
type ArrayConfig struct {
	Rows  int           `yaml:"rows"`
	Cols  int           `yaml:"columns"`
	Cores []CoreProgram `yaml:"cores"`
}

// YAMLRoot represents the root structure of the YAML file
type YAMLRoot struct {
	ArrayConfig ArrayConfig `yaml:"array_config"`
}

type Program struct {
	EntryBlocks []EntryBlock
}

type EntryBlock struct {
	EntryCond     OperandList
	CombinedInsts []CombinedInst
	Label         map[string]int
}

type CombinedInst struct {
	Insts []Inst
}

func (ci *CombinedInst) String() string {
	if len(ci.Insts) == 1 {
		return ci.Insts[0].OpCode
	} else if len(ci.Insts) > 1 {
		// return "<ADD, ADD, ADD>"
		opCodes := make([]string, len(ci.Insts))
		for i, inst := range ci.Insts {
			opCodes[i] = inst.OpCode
		}
		return "<" + strings.Join(opCodes, ", ") + ">"
	} else {
		return ""
	}
}

type Inst struct {
	// The raw text of the instruction.
	OpCode      string
	DstOperands OperandList
	SrcOperands OperandList
}

type OperandList struct {
	Operands []Operand
}

type Operand struct {
	Flag  bool
	Color string
	Impl  string
}

// LoadProgramFile loads a YAML file and converts it to a map[(x,y)]Program
func LoadProgramFile(programFilePath string) map[string]Program {
	// Read the YAML file
	data, err := os.ReadFile(programFilePath)
	if err != nil {
		panic(fmt.Sprintf("Failed to read program file: %v", err))
	}

	// Parse the YAML
	var root YAMLRoot
	err = yaml.Unmarshal(data, &root)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse YAML: %v", err))
	}

	config := root.ArrayConfig

	// Debug: Print the parsed config
	fmt.Printf("Debug: Parsed config - Rows: %d, Cols: %d, Cores: %d\n", config.Rows, config.Cols, len(config.Cores))

	// Convert to map[(x,y)]Program
	programMap := make(map[string]Program)

	for _, core := range config.Cores {
		// Create coordinate key
		coordKey := fmt.Sprintf("(%d,%d)", core.Column, core.Row)
		fmt.Printf("Debug: Processing core at %s with %d entries\n", coordKey, len(core.Entries))

		// Convert core entries to Program structure
		var entryBlocks []EntryBlock
		for _, entry := range core.Entries {
			entryBlock := EntryBlock{
				Label: make(map[string]int),
			}

			// Convert instructions
			var combinedInsts []CombinedInst
			for _, inst := range entry.Instructions {
				// Convert source operands
				var srcOperands []Operand
				for _, src := range inst.SrcOperands {
					srcOperands = append(srcOperands, Operand{
						Flag:  false, // Default flag value
						Color: src.Color,
						Impl:  src.Operand,
					})
				}

				// Convert destination operands
				var dstOperands []Operand
				for _, dst := range inst.DstOperands {
					dstOperands = append(dstOperands, Operand{
						Flag:  false, // Default flag value
						Color: dst.Color,
						Impl:  dst.Operand,
					})
				}

				// Create instruction
				instruction := Inst{
					OpCode:      inst.OpCode,
					SrcOperands: OperandList{Operands: srcOperands},
					DstOperands: OperandList{Operands: dstOperands},
				}

				// Create combined instruction
				combinedInst := CombinedInst{
					Insts: []Inst{instruction},
				}

				combinedInsts = append(combinedInsts, combinedInst)
			}

			entryBlock.CombinedInsts = combinedInsts
			entryBlocks = append(entryBlocks, entryBlock)
		}

		program := Program{
			EntryBlocks: entryBlocks,
		}

		programMap[coordKey] = program
	}

	return programMap
}

func PrintProgram(program Program) {
	for _, entryBlock := range program.EntryBlocks {
		fmt.Println(entryBlock.Label)
		for _, combinedInst := range entryBlock.CombinedInsts {
			for _, inst := range combinedInst.Insts {
				fmt.Println(inst.OpCode, inst.SrcOperands, inst.DstOperands)
			}
		}
	}
}
