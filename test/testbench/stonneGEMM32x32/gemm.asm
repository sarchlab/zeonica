# Compiled II: 2
# Array Size: 32x32


PE(*,*):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [NORTH, RED] -> [SOUTH, RED] (t=0, inv_iters=0)
  MUL_ADD, [WEST, RED], [NORTH, RED], [$2] -> [$2] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  STORE, [$2], [0] (t=1, inv_iters=0)
} (idx_per_ii=1)