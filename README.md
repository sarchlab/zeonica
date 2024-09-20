# Zeonica
Zeonica is a simulator for CGRA and wafer-scale accelerators.

## ISA Definition

### Register Space

Special registers include: 

* PC: Program Counter.
* TILE_X: The X coordinate of the tile.
* TILE_Y: The Y coordinate of the tile.
* NET_RECV_N: The head of network buffer for received data. The N here is the index of the buffer. This is configurable according to the hardware. The default configuration is like this:
	* NET_RECV_NORTH: The head of the buffer from the North.
	* NET_RECV_WEST: The head of the buffer from the West.
	* NET_RECV_SOUTH: The head of the buffer from the South.
	* NET_RECV_EAST: The head of the buffer from the East.
* NET_SEND_N: The head of network buffer for data to send. The indexing must match the NET_RECV_N register.
* Color: The color of data input in to the tile. It currently could be "R", "Y", "B". You can define the meaning of each color. Example: In passthrough, "R" represent source data.


### Instructions

All instructions have 2 or 3 operands. The first operand is the destination and the second [and the third] are the sources.

Here is the instruction list


* [I/F32]_CMP_[OP]: Integer/F32 greater than comparison. Supported OPs include:
	* EQ: Equal
	* NE: Not equal
	* LT: Less than
	* LE: Less than or equal
	* GT: Greater than
	* GE: Greater than or equal
* *LD*: Load a 32-bit value from memory. **Not developed yet.**
* *ST*: Store a 32-bit value to memory. **Not developed yet.**
* WAIT: Wait for data to receive from the network. The source must be NET_RECV_N.
* SEND: Send the data to neighbor tile. The destination must be NET_SEND_N
* JMP: Jump unconditionally.
* JEQ: Jump if equal.
* JNE: Jump if not equal.
* *DONE*: Do nothing, waste one cycle.(Seems not useful now.)
* MAC: Multiply and accumulate. Y=ab+Y Systolic array instruction
* ADDI: Integer addition. The third argument could be immediate number or register.
* *CONFIG_ROUTING*：Add the sending data into routing queue.(Seems not useful now.)
* *TRIGGER_SEND*: If a specific data is coming to current tile, it will send out the data it need to send out.(Seems not useful now.)
* TRIGGER_TWO_SIDE: If the data from both two sides are ready, trigger specific code block. Besides, add the trigger into trigger queue for SLEEP.
* TRIGGER_ONE_SIDE: If the data from one side are ready, trigger specific code block. Besides, add the trigger into trigger queue for SLEEP.
* *IDLE*: Idle one cycle, PC+1.
* RECV_SEND:Receive, store in register and send the data in one cycle.
* SLEEP:Try each trigger stored in trigger queue. If one is triggered, jump to the specific block queue. Otherwise, keep trying. PC will not add 1.

### Example: Pass-through left to right

```assembly
	WAIT, $0, NET_RECV_3
	SEND, $0, NET_SEND_1
```

### Example: ReLU


```assembly
	WAIT, $0, NET_RECV_3
	F_CMD_EQ, $1, $0, 0
	JEQ, ELSE, $1, 1
IF:
	SEND, NET_SEND_1, $0
	JMP, END
ELSE:
	SEND, NET_SEND_1, 0
END:
	DONE,
```

### Example: Matrix  Multiplication
```assembly
START:
    TRIGGER_TWO_SIDE, COMPUTE, WEST_R, NORTH_R
    TRIGGER_ONE_SIDE, SHIFT, NORTH_B 
    SLEEP
COMPUTE:
    RECV_SEND, SOUTH_R, $1, NORTH_R
    RECV_SEND, EAST_R, $2, WEST_R
    MAC, $3, $1, $2
    SLEEP
SHIFT:
    RECV_SEND, SOUTH_B, $3, NORTH_B
    SLEEP
```
 
 ## Configuration
- The Zeonica simulator is developed based on [Golang](https://go.dev/) 1.16. Please install Golang 1.16 or later version.
- The Zeonica uses [Akita](https://github.com/sarchlab/akita) simulator engine. If you want to use Zeonica offline, please download the Akita and modify the two replace in go.mod into proper directory. 
### Run Examples
1. Go to specific example directory. (For first time running, you need to download dependencies, which are automatically downloaded.)
2. Run `go build` to compile and build the executable file.
3. Run `./EXAMPLE`
### Hierarchy of Example
- Assembly codes: EXAMPLE.cgraasm
- main.go
	- Define the CGRA size
	- Read assembly codes
	- Input data preprocessing
	- Feed in input data and set up collect
	- Map assembly code to each tile
	- Run the engine
	- Print out the collected data