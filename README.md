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
* @: @ symbol is a decorator, which will trigger some procedure. These procedures are not controlled by clock.

### Instructions

All instructions have 2 or 3 operands. The first operand is the destination and the second [and the third] are the sources.

Here is the instruction list

* @Wait_AND: Wait data from two or more directions are ready, otherwise jump to the bottom of the code.
* @ROUTER_FORWARD: Send the data from the opposite direction where the data is from. 
* I_ADD: Integer addition.
* MAC: Multiply and accumulate.
* [I/F32]_CMP_[OP]: Integer/F32 greater than comparison. Supported OPs include:
	* EQ: Equal
	* NE: Not equal
	* LT: Less than
	* LE: Less than or equal
	* GT: Greater than
	* GE: Greater than or equal
* LD: Load a 32-bit value from memory.
* ST: Store a 32-bit value to memory.
* WAIT: Wait for data to receive from the network. The source must be NET_RECV_N.
* JEQ: Jump if equal.
* JMP: Jump unconditionally.

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
 