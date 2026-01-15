package core

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
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
	IndexPerII int             `yaml:"index_per_ii"`
}

type YAMLOperation struct {
	OpCode            string        `yaml:"opcode"`
	SrcOperands       []YAMLOperand `yaml:"src_operands"`
	DstOperands       []YAMLOperand `yaml:"dst_operands"`
	ID                int           `yaml:"id"`
	InvalidIterations int           `yaml:"invalid_iterations"`
	TimeStep          int           `yaml:"time_step"`
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
	OpCode            string
	DstOperands       OperandList
	SrcOperands       OperandList
	ID                int // ID from YAML file
	InvalidIterations int // Invalid iterations from YAML file
}

type OperandList struct {
	Operands []Operand
}

type Operand struct {
	Flag  bool
	Color string
	Impl  string
}

// LoadProgramFileFromYAML loads a YAML file and converts it to a map[(x,y)]Program
func LoadProgramFileFromYAML(programFilePath string) map[string]Program {
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
						OpCode:            yamlOp.OpCode,
						SrcOperands:       OperandList{Operands: srcOperands},
						DstOperands:       OperandList{Operands: dstOperands},
						ID:                yamlOp.ID,
						InvalidIterations: yamlOp.InvalidIterations,
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

// splitRespectingBrackets splits a string by delimiter, but respects brackets
// so [WEST, RED] is treated as a single token
func splitRespectingBrackets(s, delimiter string) []string {
	var result []string
	var current strings.Builder
	bracketDepth := 0

	for _, char := range s {
		if char == '[' {
			bracketDepth++
			current.WriteRune(char)
		} else if char == ']' {
			bracketDepth--
			current.WriteRune(char)
		} else if char == rune(delimiter[0]) && bracketDepth == 0 {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// parseASMOperand parses an operand string from ASM format and returns an Operand.
// Supports formats:
//   - [NORTH, RED], [WEST, YELLOW], [EAST, BLUE] -> direction + color
//   - [$0], [$1] -> register
//   - [#0], [#1], [114] -> immediate value (with or without #)
//   - North, East, South, West -> direction only
//   - $0, $1 -> register (without brackets)
//   - 114, 514 -> immediate value (without brackets)
func parseASMOperand(opStr string) Operand {
	opStr = strings.TrimSpace(opStr)

	// Remove brackets if present
	if strings.HasPrefix(opStr, "[") && strings.HasSuffix(opStr, "]") {
		opStr = opStr[1 : len(opStr)-1]
		opStr = strings.TrimSpace(opStr)
	}

	// Parse [NORTH, RED] format
	parts := strings.Split(opStr, ",")
	if len(parts) == 2 {
		direction := strings.TrimSpace(parts[0])
		color := strings.TrimSpace(parts[1])

		// Normalize direction: NORTH -> North, etc.
		direction = strings.Title(strings.ToLower(direction))
		if direction == "North" || direction == "South" || direction == "East" || direction == "West" {
			// Normalize color: RED -> R, YELLOW -> Y, BLUE -> B
			colorNormalized := ""
			colorUpper := strings.ToUpper(color)
			switch colorUpper {
			case "RED":
				colorNormalized = "R"
			case "YELLOW":
				colorNormalized = "Y"
			case "BLUE":
				colorNormalized = "B"
			default:
				colorNormalized = colorUpper
			}

			return Operand{
				Flag:  false,
				Color: colorNormalized,
				Impl:  direction,
			}
		}
	}

	// Parse register: $0, $1, etc.
	if strings.HasPrefix(opStr, "$") {
		return Operand{
			Flag:  false,
			Color: "",
			Impl:  opStr,
		}
	}

	// Parse immediate: #0, #1, or just number
	if strings.HasPrefix(opStr, "#") {
		return Operand{
			Flag:  false,
			Color: "",
			Impl:  opStr[1:], // Remove # prefix
		}
	}

	// Parse direction only: North, East, South, West
	direction := strings.Title(strings.ToLower(opStr))
	if direction == "North" || direction == "South" || direction == "East" || direction == "West" {
		return Operand{
			Flag:  false,
			Color: "",
			Impl:  direction,
		}
	}

	// Try parsing as number (immediate value)
	if _, err := strconv.Atoi(opStr); err == nil {
		return Operand{
			Flag:  false,
			Color: "",
			Impl:  opStr,
		}
	}

	// Default: treat as string operand
	return Operand{
		Flag:  false,
		Color: "",
		Impl:  opStr,
	}
}

// parseASMInstruction parses a single instruction line from ASM format.
// Format: OPCODE, [src1], [src2], ... -> [dst1], [dst2], ...
// Or: OPCODE [src1] [src2] ... -> [dst1] [dst2] ...
func parseASMInstruction(line string) (string, []Operand, []Operand) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, nil
	}

	// Split by -> to separate source and destination operands
	parts := strings.Split(line, "->")
	if len(parts) != 2 {
		// No -> means no destination operands
		parts = []string{line, ""}
	}

	srcPart := strings.TrimSpace(parts[0])
	dstPart := strings.TrimSpace(parts[1])

	// Extract opcode from source part
	// Opcode is before the first operand
	var opcode string
	var srcOperands []Operand

	// Try comma-separated format first, but respect brackets
	if strings.Contains(srcPart, ",") {
		// Split by comma, but handle brackets carefully
		fields := splitRespectingBrackets(srcPart, ",")
		if len(fields) > 0 {
			opcode = strings.TrimSpace(fields[0])
			for i := 1; i < len(fields); i++ {
				op := parseASMOperand(strings.TrimSpace(fields[i]))
				srcOperands = append(srcOperands, op)
			}
		}
	} else {
		// Space-separated format
		fields := strings.Fields(srcPart)
		if len(fields) > 0 {
			opcode = fields[0]
			for i := 1; i < len(fields); i++ {
				op := parseASMOperand(fields[i])
				srcOperands = append(srcOperands, op)
			}
		} else {
			opcode = srcPart
		}
	}

	// Parse destination operands
	var dstOperands []Operand
	if dstPart != "" {
		// Try comma-separated format, but respect brackets
		if strings.Contains(dstPart, ",") {
			dstFields := splitRespectingBrackets(dstPart, ",")
			for _, field := range dstFields {
				op := parseASMOperand(strings.TrimSpace(field))
				dstOperands = append(dstOperands, op)
			}
		} else {
			// Space-separated format
			fields := strings.Fields(dstPart)
			for _, field := range fields {
				op := parseASMOperand(field)
				dstOperands = append(dstOperands, op)
			}
		}
	}

	return opcode, srcOperands, dstOperands
}

// parsePEFormat parses PE(x,y): format ASM file
func parsePEFormat(content string) map[string]Program {
	programMap := make(map[string]Program)
	lines := strings.Split(content, "\n")

	peRegex := regexp.MustCompile(`PE\((\d+),(\d+)\):`)
	var currentCoreX, currentCoreY int = -1, -1
	var currentEntryBlock *EntryBlock
	var currentInstructionGroup *InstructionGroup
	inInstructionGroup := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for PE(x,y): header
		matches := peRegex.FindStringSubmatch(line)
		if matches != nil {
			// Save previous core if exists
			if currentCoreX >= 0 && currentEntryBlock != nil {
				coordKey := fmt.Sprintf("(%d,%d)", currentCoreX, currentCoreY)
				programMap[coordKey] = Program{
					EntryBlocks: []EntryBlock{*currentEntryBlock},
				}
			}

			// Parse coordinates
			currentCoreX, _ = strconv.Atoi(matches[1])
			currentCoreY, _ = strconv.Atoi(matches[2])

			// Initialize new entry block for this core
			currentEntryBlock = &EntryBlock{
				Label:             make(map[string]int),
				InstructionGroups: []InstructionGroup{},
			}
			currentInstructionGroup = nil
			inInstructionGroup = false
			continue
		}

		// Check for instruction group start {
		if strings.HasPrefix(line, "{") {
			currentInstructionGroup = &InstructionGroup{
				Operations: []Operation{},
				RefCount:   make(map[string]int),
			}
			inInstructionGroup = true

			// Remove opening brace
			line = strings.TrimSpace(strings.TrimPrefix(line, "{"))
			// Continue parsing the rest of the line
		}

		// Check for instruction group end } (possibly with timestamp)
		if strings.Contains(line, "}") {
			// Extract content before }
			beforeBrace := strings.Split(line, "}")[0]
			beforeBrace = strings.TrimSpace(beforeBrace)

			// Parse any instructions on this line before }
			if beforeBrace != "" && inInstructionGroup && currentInstructionGroup != nil {
				opcode, srcOps, dstOps := parseASMInstruction(beforeBrace)
				if opcode != "" {
					// Update RefCount for source operands that are network ports
					for _, srcOp := range srcOps {
						if srcOp.Color != "" && (srcOp.Impl == "North" || srcOp.Impl == "South" || srcOp.Impl == "East" || srcOp.Impl == "West") {
							key := srcOp.Impl + srcOp.Color
							currentInstructionGroup.RefCount[key]++
						}
					}

					operation := Operation{
						OpCode:      opcode,
						SrcOperands: OperandList{Operands: srcOps},
						DstOperands: OperandList{Operands: dstOps},
					}
					currentInstructionGroup.Operations = append(currentInstructionGroup.Operations, operation)
				}
			}

			// Close instruction group
			if currentInstructionGroup != nil && len(currentInstructionGroup.Operations) > 0 {
				if currentEntryBlock != nil {
					currentEntryBlock.InstructionGroups = append(currentEntryBlock.InstructionGroups, *currentInstructionGroup)
				}
			}
			currentInstructionGroup = nil
			inInstructionGroup = false
			continue
		}

		// Parse instruction within instruction group
		if inInstructionGroup && currentInstructionGroup != nil {
			opcode, srcOps, dstOps := parseASMInstruction(line)
			if opcode != "" {
				// Update RefCount for source operands that are network ports
				for _, srcOp := range srcOps {
					if srcOp.Color != "" && (srcOp.Impl == "North" || srcOp.Impl == "South" || srcOp.Impl == "East" || srcOp.Impl == "West") {
						key := srcOp.Impl + srcOp.Color
						currentInstructionGroup.RefCount[key]++
					}
				}

				operation := Operation{
					OpCode:      opcode,
					SrcOperands: OperandList{Operands: srcOps},
					DstOperands: OperandList{Operands: dstOps},
				}
				currentInstructionGroup.Operations = append(currentInstructionGroup.Operations, operation)
			}
		} else if !inInstructionGroup {
			// Handle case where line doesn't start with { but should be in a group
			// This shouldn't happen in PE format, but handle gracefully
			opcode, srcOps, dstOps := parseASMInstruction(line)
			if opcode != "" {
				// Create a single-instruction group
				instructionGroup := InstructionGroup{
					Operations: []Operation{},
					RefCount:   make(map[string]int),
				}

				// Update RefCount
				for _, srcOp := range srcOps {
					if srcOp.Color != "" && (srcOp.Impl == "North" || srcOp.Impl == "South" || srcOp.Impl == "East" || srcOp.Impl == "West") {
						key := srcOp.Impl + srcOp.Color
						instructionGroup.RefCount[key]++
					}
				}

				operation := Operation{
					OpCode:      opcode,
					SrcOperands: OperandList{Operands: srcOps},
					DstOperands: OperandList{Operands: dstOps},
				}
				instructionGroup.Operations = append(instructionGroup.Operations, operation)

				if currentEntryBlock != nil {
					currentEntryBlock.InstructionGroups = append(currentEntryBlock.InstructionGroups, instructionGroup)
				}
			}
		}
	}

	// Save last core
	if currentCoreX >= 0 && currentEntryBlock != nil {
		coordKey := fmt.Sprintf("(%d,%d)", currentCoreX, currentCoreY)
		programMap[coordKey] = Program{
			EntryBlocks: []EntryBlock{*currentEntryBlock},
		}
	}

	return programMap
}

// parseCoreFormat parses Core x,y: format ASM file
func parseCoreFormat(content string) map[string]Program {
	programMap := make(map[string]Program)
	lines := strings.Split(content, "\n")

	coreRegex := regexp.MustCompile(`Core\s+(\d+),(\d+):`)
	var currentCoreX, currentCoreY int = -1, -1
	var currentEntryBlock *EntryBlock

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for Core x,y: header
		matches := coreRegex.FindStringSubmatch(line)
		if matches != nil {
			// Save previous core if exists
			if currentCoreX >= 0 && currentEntryBlock != nil {
				coordKey := fmt.Sprintf("(%d,%d)", currentCoreX, currentCoreY)
				programMap[coordKey] = Program{
					EntryBlocks: []EntryBlock{*currentEntryBlock},
				}
			}

			// Parse coordinates
			currentCoreX, _ = strconv.Atoi(matches[1])
			currentCoreY, _ = strconv.Atoi(matches[2])

			// Initialize new entry block for this core
			currentEntryBlock = &EntryBlock{
				Label:             make(map[string]int),
				InstructionGroups: []InstructionGroup{},
			}
			continue
		}

		// Parse instruction line
		if currentEntryBlock != nil {
			opcode, srcOps, dstOps := parseASMInstruction(line)
			if opcode != "" {
				// Create a single-instruction group for this instruction
				instructionGroup := InstructionGroup{
					Operations: []Operation{},
					RefCount:   make(map[string]int),
				}

				// Update RefCount for source operands that are network ports
				for _, srcOp := range srcOps {
					if srcOp.Color != "" && (srcOp.Impl == "North" || srcOp.Impl == "South" || srcOp.Impl == "East" || srcOp.Impl == "West") {
						key := srcOp.Impl + srcOp.Color
						instructionGroup.RefCount[key]++
					} else if srcOp.Impl == "North" || srcOp.Impl == "South" || srcOp.Impl == "East" || srcOp.Impl == "West" {
						// Direction without color - still count it
						key := srcOp.Impl
						instructionGroup.RefCount[key]++
					}
				}

				operation := Operation{
					OpCode:      opcode,
					SrcOperands: OperandList{Operands: srcOps},
					DstOperands: OperandList{Operands: dstOps},
				}
				instructionGroup.Operations = append(instructionGroup.Operations, operation)
				currentEntryBlock.InstructionGroups = append(currentEntryBlock.InstructionGroups, instructionGroup)
			}
		}
	}

	// Save last core
	if currentCoreX >= 0 && currentEntryBlock != nil {
		coordKey := fmt.Sprintf("(%d,%d)", currentCoreX, currentCoreY)
		programMap[coordKey] = Program{
			EntryBlocks: []EntryBlock{*currentEntryBlock},
		}
	}

	return programMap
}

// LoadProgramFileFromASM loads an ASM file and converts it to a map[(x,y)]Program
func LoadProgramFileFromASM(programFilePath string) map[string]Program {
	// Read the ASM file
	data, err := os.ReadFile(programFilePath)
	if err != nil {
		panic(fmt.Sprintf("Failed to read ASM file: %v", err))
	}

	content := string(data)

	// Detect format: PE(x,y): or Core x,y:
	if strings.Contains(content, "PE(") {
		return parsePEFormat(content)
	} else if strings.Contains(content, "Core ") {
		return parseCoreFormat(content)
	} else {
		panic(fmt.Sprintf("Unsupported ASM format in file: %s", programFilePath))
	}
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
