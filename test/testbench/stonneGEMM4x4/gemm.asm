# Compiled II: 2
# TPU-like GEMM (operand-driven, simplified)
# A tokens come from WEST, B tokens come from NORTH
# Forward A/B tokens to EAST/SOUTH in the same bundle as MUL_ADD
# $2 is local accumulator (psum), assumed initialized before execution
# NOTE: STORE is unconditional (writes partial sums repeatedly)

PE(0,0):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(1,0):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(2,0):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(3,0):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(0,1):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(1,1):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(2,1):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(3,1):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(0,2):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(1,2):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(2,2):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(3,2):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(0,3):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(1,3):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(2,3):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)

PE(3,3):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)