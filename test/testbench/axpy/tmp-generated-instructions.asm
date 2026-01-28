# Compiled II: 6

PE(0,0):
{
  CONSTANT, [arg0] -> [$0] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  ICMP_SGT, [$0], [#0] -> [$0], [$1] (t=1, inv_iters=0)
} (idx_per_ii=1)
{
  GRANT_ONCE, [$0] -> [$0], [EAST, RED], [NORTH, RED] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  DATA_MOV, [$0] -> [EAST, RED] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  NOT, [$1] -> [NORTH, RED] (t=5, inv_iters=0)
} (idx_per_ii=5)

PE(1,0):
{
  ZEXT, [$0] -> [NORTH, RED] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  GRANT_PREDICATE, [EAST, RED], [WEST, RED] -> [$0] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  PHI_START, [$0], [NORTH, RED] -> [EAST, RED], [$2], [NORTH, RED] (t=4, inv_iters=0)
  DATA_MOV, [EAST, RED] -> [$0] (t=4, inv_iters=0)
  DATA_MOV, [WEST, RED] -> [$1] (t=4, inv_iters=0)
} (idx_per_ii=4)
{
  GRANT_PREDICATE, [$0], [$1] -> [$0] (t=5, inv_iters=0)
  DATA_MOV, [$2] -> [EAST, RED] (t=5, inv_iters=0)
} (idx_per_ii=5)

PE(2,0):
{
  GEP, [WEST, RED] -> [$0], [NORTH, RED] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  LOAD, [$0] -> [NORTH, RED] (t=7, inv_iters=1)
} (idx_per_ii=1)
{
  GRANT_ONCE, [#0] -> [WEST, RED] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  GRANT_ONCE, [arg0] -> [WEST, RED] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  GEP, [WEST, RED] -> [NORTH, RED] (t=5, inv_iters=0)
} (idx_per_ii=5)

PE(0,1):
{
  GRANT_ONCE, [SOUTH, RED] -> [$0] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  GRANT_PREDICATE, [$0], [EAST, RED] -> [$0] (t=7, inv_iters=1)
} (idx_per_ii=1)
{
  DATA_MOV, [SOUTH, RED] -> [EAST, RED] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  PHI, [$0], [EAST, RED] -> [NORTH, RED] (t=11, inv_iters=1)
} (idx_per_ii=5)

PE(1,1):
{
  NOT, [$1] -> [WEST, RED] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  ICMP_EQ, [$0], [SOUTH, RED] -> [$0], [$1], [$2] (t=7, inv_iters=1)
} (idx_per_ii=1)
{
  NOT, [$0] -> [$0] (t=8, inv_iters=1)
} (idx_per_ii=2)
{
  GRANT_PREDICATE, [$3], [$0] -> [SOUTH, RED] (t=9, inv_iters=1)
} (idx_per_ii=3)
{
  DATA_MOV, [WEST, RED] -> [$1] (t=4, inv_iters=0)
  GRANT_PREDICATE, [$1], [$2] -> [WEST, RED] (t=10, inv_iters=1)
} (idx_per_ii=4)
{
  ADD, [SOUTH, RED], [#1] -> [$0], [$3] (t=5, inv_iters=0)
} (idx_per_ii=5)

PE(2,1):
{
  LOAD, [SOUTH, RED] -> [$0] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  MUL, [$0], [arg1] -> [$0] (t=7, inv_iters=1)
  DATA_MOV, [SOUTH, RED] -> [$1] (t=7, inv_iters=1)
} (idx_per_ii=1)
{
  ADD, [$0], [SOUTH, RED] -> [$0] (t=8, inv_iters=1)
} (idx_per_ii=2)
{
  STORE, [$0], [$1] (t=9, inv_iters=1)
} (idx_per_ii=3)

PE(0,2):
{
  RETURN_VOID, [SOUTH, RED] (t=12, inv_iters=2)
} (idx_per_ii=0)

