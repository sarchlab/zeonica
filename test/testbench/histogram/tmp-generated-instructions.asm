# Compiled II: 6

PE(0,0):
{
  GRANT_ONCE, [#0] -> [EAST, RED] (t=0, inv_iters=0)
} (idx_per_ii=0)
{
  ADD, [EAST, RED], [#-5] -> [NORTH, RED] (t=5, inv_iters=0)
} (idx_per_ii=5)

PE(1,0):
{
  CTRL_MOV, [NORTH, RED] -> [$0] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  PHI_START, [WEST, RED], [$0] -> [$0], [NORTH, RED] (t=1, inv_iters=0)
} (idx_per_ii=1)
{
  GEP, [arg0], [$0] -> [$0] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  LOAD, [$0] -> [$0] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  MUL, [$0], [#5] -> [WEST, RED] (t=4, inv_iters=0)
} (idx_per_ii=4)

PE(0,1):
{
  DIV, [SOUTH, RED], [#18] -> [$0] (t=6, inv_iters=1)
} (idx_per_ii=0)
{
  SEXT, [$0] -> [$0] (t=7, inv_iters=1)
} (idx_per_ii=1)
{
  GEP, [arg1], [$0] -> [$0], [$1] (t=8, inv_iters=1)
} (idx_per_ii=2)
{
  LOAD, [$0] -> [$0] (t=9, inv_iters=1)
} (idx_per_ii=3)
{
  ADD, [$0], [#1] -> [$0] (t=10, inv_iters=1)
} (idx_per_ii=4)
{
  STORE, [$0], [$1] (t=11, inv_iters=1)
} (idx_per_ii=5)

PE(1,1):
{
  ADD, [SOUTH, RED], [#1] -> [$0], [$1] (t=2, inv_iters=0)
} (idx_per_ii=2)
{
  ICMP_EQ, [$0], [#20] -> [$0], [NORTH, RED], [$2] (t=3, inv_iters=0)
} (idx_per_ii=3)
{
  NOT, [$0] -> [$0] (t=4, inv_iters=0)
  DATA_MOV, [$2] -> [NORTH, RED] (t=4, inv_iters=0)
} (idx_per_ii=4)
{
  GRANT_PREDICATE, [$1], [$0] -> [SOUTH, RED] (t=5, inv_iters=0)
} (idx_per_ii=5)

PE(1,2):
{
  DATA_MOV, [SOUTH, RED] -> [$0] (t=4, inv_iters=0)
  GRANT_PREDICATE, [$0], [$1] -> [$1] (t=10, inv_iters=1)
} (idx_per_ii=4)
{
  DATA_MOV, [SOUTH, RED] -> [$1] (t=5, inv_iters=0)
  RETURN_VOID, [$1] (t=11, inv_iters=1)
} (idx_per_ii=5)

