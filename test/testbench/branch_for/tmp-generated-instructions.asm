# Compiled II: 4

PE(0,0):
{
  GRANT_ONCE, [#0] -> [EAST, RED] (t=0, inv_iters=0)
  DATA_MOV, [EAST, RED] -> [NORTH, RED] (t=4, inv_iters=1)
} (idx_per_ii=0)
{
  GRANT_ONCE, [#0.000000] -> [$0] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  PHI_START, [$0], [NORTH, RED] -> [NORTH, RED] (t=3, inv_iters=0)
} (idx_per_ii=3)

PE(1,0):
{
  GRANT_PREDICATE, [$1], [$0] -> [$0] (t=4, inv_iters=1)
} (idx_per_ii=0)
{
  PHI_START, [WEST, RED], [$0] -> [$0] (t=1, inv_iters=0)
  DATA_MOV, [EAST, RED] -> [NORTH, RED] (t=5, inv_iters=1)
} (idx_per_ii=1)
{
  ADD, [$0], [#1] -> [$0], [$1] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  ICMP_SLT, [$0], [#10] -> [$0], [WEST, RED], [EAST, RED] (t=3, inv_iters=0)
} (idx_per_ii=3)

PE(2,0):
{
  NOT, [WEST, RED] -> [WEST, RED] (t=4, inv_iters=1)
} (idx_per_ii=0)

PE(0,1):
{
  FADD, [SOUTH, RED], [#3.000000] -> [$0], [EAST, RED] (t=4, inv_iters=1)
} (idx_per_ii=0)
{
  DATA_MOV, [SOUTH, RED] -> [$1] (t=5, inv_iters=1)
} (idx_per_ii=1)
{
  GRANT_PREDICATE, [$0], [$1] -> [SOUTH, RED] (t=6, inv_iters=1)
} (idx_per_ii=2)

PE(1,1):
{
  DATA_MOV, [WEST, RED] -> [$0] (t=5, inv_iters=1)
} (idx_per_ii=1)
{
  GRANT_PREDICATE, [$0], [SOUTH, RED] -> [$0] (t=6, inv_iters=1)
} (idx_per_ii=2)
{
  RETURN_VALUE, [$0] (t=7, inv_iters=1)
} (idx_per_ii=3)

