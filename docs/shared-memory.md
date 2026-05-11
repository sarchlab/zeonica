# Shared Memory

Zeonica supports shared memory through `memory_mode: "shared"` in the
architecture spec. Shared-memory tiles are assigned to a shared-memory group by
`shared_memory_groups`; tiles in the same group see the same backing storage.

## Models

The shared-memory model is selected under `simulator.device`:

```yaml
memory_mode: "shared"
shared_memory_model: "ideal"   # ideal | banked
shared_memory_banks: 2
shared_memory_base_latency: 1
shared_memory_bank_interleave_bytes: 4
```

- `ideal`: shared storage without modeled bank conflicts.
- `banked`: shared SRAM scratchpad with per-bank serialization.

The `banked` model is intended to represent CGRA-adjacent shared SRAM, not an
external DRAM controller. Compiler-style blocking `LOAD` and `STORE` use a
direct SRAM path for timing and storage access instead of sending Akita
`mem.ReadReq` or `mem.WriteReq` messages through the router.

## Bank Timing

Bank selection uses byte addresses:

```text
phys_addr = shared_memory_base + program_addr
byte_addr = phys_addr * 4
bank = (byte_addr / shared_memory_bank_interleave_bytes) % shared_memory_banks
```

Each bank tracks its next available cycle:

```text
done_cycle = max(issue_cycle, next_bank_cycle[bank]) + shared_memory_base_latency
next_bank_cycle[bank] = done_cycle
```

With `shared_memory_base_latency: 1`, a request with no conflict completes on
the next cycle. Multiple same-cycle requests to the same bank complete one per
cycle; requests to different banks can complete independently.

## Instruction Forms

Compiler-style shared-memory operations do not need `LDW` or `STW`:

```yaml
- opcode: "LOAD"
  src_operands:
    - operand: "WEST"   # address
      color: "RED"
  dst_operands:
    - operand: "$0"     # register or direction port
      color: "RED"

- opcode: "STORE"
  src_operands:
    - operand: "#99"    # value
      color: "RED"
    - operand: "#8"     # address
      color: "RED"
  dst_operands: []
```

In shared-memory mode, these operations block internally until the SRAM
operation reaches `done_cycle`. `LOAD` then writes the returned value to the
destination register or direction port; `STORE` writes to shared storage before
the PC advances.

The older `LOAD -> Router` plus `LDW` and `STORE -> Router` plus `STW` forms are
still kept for compatibility and continue to use the existing request/response
path.

## BiCG Shared-Memory Test

The shared-memory BiCG configuration is:

```text
test/arch_spec/arch_spec_bicg_shared_banked_boundary.yaml
```

Run it with:

```bash
ZEONICA_ARCH_SPEC=test/arch_spec/arch_spec_bicg_shared_banked_boundary.yaml \
  go run ./test/testbench/bicg
```

The trace is written to:

```text
test/testbench/trace/bicg.json.log
```

For the current BiCG fixture, a representative same-bank conflict appears as
three `LOAD` requests issued in cycle 7 and completed in cycles 8, 9, and 10.
