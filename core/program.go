package core

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CoreProgram represents a program for a specific core
type YAMLCoreProgram struct {
	Row     int         `yaml:"row"`
	Column  int         `yaml:"column"`
	CoreID  string      `yaml:"core_id"`
	Entries []YAMLEntry `yaml:"entries"`
}

// YAMLEntry represents an entry block in the YAML
type YAMLEntry struct {
	EntryID           string                 `yaml:"entry_id"`
	Type              string                 `yaml:"type"`
	InstructionGroups []YAMLInstructionGroup `yaml:"instructions"`
}

// Instruction represents a single instruction in the YAML
type YAMLInstructionGroup struct {
	Operations []YAMLOperation `yaml:"operations"`
}

type YAMLOperation struct {
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
	Rows  int               `yaml:"rows"`
	Cols  int               `yaml:"columns"`
	Cores []YAMLCoreProgram `yaml:"cores"`
}

// YAMLRoot represents the root structure of the YAML file
type YAMLRoot struct {
	ArrayConfig ArrayConfig `yaml:"array_config"`
}

type Program struct {
	EntryBlocks []EntryBlock
}

type EntryBlock struct {
	EntryCond         OperandList // not used
	InstructionGroups []InstructionGroup
	Label             map[string]int
}

type InstructionGroup struct {
	Operations []Operation
	RefCount   map[string]int
}

func (ig *InstructionGroup) String() string {
	if len(ig.Operations) == 1 {
		return ig.Operations[0].OpCode
	} else if len(ig.Operations) > 1 {
		// return "<ADD, ADD, ADD>"
		opCodes := make([]string, len(ig.Operations))
		for i, operation := range ig.Operations {
			opCodes[i] = operation.OpCode
		}
		return "<" + strings.Join(opCodes, ", ") + ">"
	} else {
		return ""
	}
}

type Operation struct {
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

			// Convert instruction groups
			var instructionGroups []InstructionGroup
			for _, instGroup := range entry.InstructionGroups {
				instructionGroup := InstructionGroup{
					RefCount: make(map[string]int),
				}

				// Convert operations
				var operations []Operation
				for _, yamlOp := range instGroup.Operations {
					// Convert source operands
					var srcOperands []Operand
					for _, src := range yamlOp.SrcOperands {
						srcOperands = append(srcOperands, Operand{
							Flag:  false, // Default flag value
							Color: src.Color,
							Impl:  src.Operand,
						})
						instructionGroup.RefCount[src.Operand+src.Color]++
					}

					// Convert destination operands
					var dstOperands []Operand
					for _, dst := range yamlOp.DstOperands {
						dstOperands = append(dstOperands, Operand{
							Flag:  false, // Default flag value
							Color: dst.Color,
							Impl:  dst.Operand,
						})
					}

					// Create operation
					operation := Operation{
						OpCode:      yamlOp.OpCode,
						SrcOperands: OperandList{Operands: srcOperands},
						DstOperands: OperandList{Operands: dstOperands},
					}

					operations = append(operations, operation)
				}

				instructionGroup.Operations = operations
				instructionGroups = append(instructionGroups, instructionGroup)
			}

			entryBlock.InstructionGroups = instructionGroups
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
		for _, instructionGroup := range entryBlock.InstructionGroups {
			for _, operation := range instructionGroup.Operations {
				fmt.Println(operation.OpCode, operation.SrcOperands, operation.DstOperands)
			}
		}
	}
}
