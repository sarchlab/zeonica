# Compiled II: 5

PE(2,1):
{
  GRANT_PREDICATE, [$0], [NORTH, RED] -> [$0] (t=5, inv_iters=1)
} (idx_per_ii=0)
{
  RETURN_VOID, [$0] (t=6, inv_iters=1)
} (idx_per_ii=1)
{
  DATA_MOV, [NORTH, RED] -> [$0] (t=4, inv_iters=0)
} (idx_per_ii=4)

PE(2,2):
{
  GRANT_PREDICATE, [$1], [$0] -> [$0] (t=5, inv_iters=1)
} (idx_per_ii=0)
{
  PHI_START, [EAST, RED], [$0] -> [EAST, RED], [NORTH, RED], [$0] (t=1, inv_iters=0)
} (idx_per_ii=1)
{
  ADD, [$0], [#1] -> [$0], [$1] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  ICMP_EQ, [$0], [#16] -> [$0], [SOUTH, RED], [$2] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  NOT, [$0] -> [$0] (t=4, inv_iters=0)
  DATA_MOV, [$2] -> [SOUTH, RED] (t=4, inv_iters=0)
} (idx_per_ii=4)

PE(3,2):
{
  GRANT_ONCE, [#0] -> [WEST, RED] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  GEP, [WEST, RED] -> [$0] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  LOAD, [$0] -> [$0] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  MUL, [$0], [#3] -> [NORTH, RED] (t=4, inv_iters=0)
} (idx_per_ii=4)

PE(2,3):
{
  DATA_MOV, [SOUTH, RED] -> [$0] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  GEP, [$0] -> [$0], [EAST, RED] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  LOAD, [$0] -> [EAST, RED] (t=4, inv_iters=0)
} (idx_per_ii=4)

PE(3,3):
{
  ADD, [SOUTH, RED], [WEST, RED] -> [$0] (t=5, inv_iters=1)
} (idx_per_ii=0)
{
  STORE, [$0], [$1] (t=6, inv_iters=1)
} (idx_per_ii=1)
{
  DATA_MOV, [WEST, RED] -> [$1] (t=4, inv_iters=0)
} (idx_per_ii=4)

