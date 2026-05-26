# Verify 包说明

本文档说明 Zeonica 中 `verify` 文件夹的职责、组成和用法。

---

## 一句话定位

**verify** 是 Zeonica 的**快速验证层**：在跑 cycle-accurate 仿真之前，用**静态检查 + 无时序的功能仿真**先检查 kernel 的**结构、时序约束和计算语义**，并生成可读的验证报告。不建模周期、网络延迟和 backpressure，只关心“对不对”，不关心“多快”。

---

## 三块功能

### 1. Lint（静态检查）— `lint.go`

**作用**：只看 kernel YAML 和架构信息，不做执行，快速发现映射/调度错误。

- **STRUCT**
  - 坐标格式是否合法（如 `"(x,y)"`）
  - PE 坐标是否在 CGRA 范围内（如 4×4 下不能有 (5,5)）
  - 同一 (PE, timestep) 内是否有**端口写冲突**（同一端口被多条指令写）
- **TIMING**
  - 跨 PE 数据依赖的**时序是否满足**
  - 规则：生产者写 → 消费者读，中间至少要经过 `距离 × HopLatency` 个周期
  - 支持 modulo scheduling（II > 0）：用 D∈{0,1} 的迭代距离模型，减少误报

**输入**：`programs map[string]core.Program`（从 kernel YAML 加载）+ `arch *ArchInfo`（行列、mesh、HopLatency 等）。  
**输出**：`[]Issue`，每个 issue 带类型（STRUCT/TIMING）、PE、timestep、op 索引、消息和 Details。

---

### 2. Functional Simulator（功能仿真）— `funcsim.go`

**作用**：按**数据依赖**执行 kernel，不建模周期、网络延迟和 backpressure，只验证**计算语义**是否正确。

- 执行顺序：按 timestep 拓扑执行，某条 op 的**所有源操作数就绪**才执行
- 每个 PE 维护：寄存器、本地 memory、端口上的“数据是否到位”
- 数据带 **predicate**（valid/invalid），运算会传播 predicate（如二元 op 的 pred = pred0 AND pred1）
- 支持 35+ opcode（与 `core/emu.go` 语义对齐的 subset：算术、逻辑、内存、比较、PHI、控制等）
- **不做**：周期推进、网络延迟、SendBuf 满时的阻塞

**输入**：同样 `programs` + `arch`，还可 `PreloadMemory(x, y, value, addr)` 预填内存。  
**输出**：执行完后用 `GetRegisterValue(x, y, regIndex)`、`GetMemoryValue(x, y, addr)` 等查结果；若执行卡死或出错，`Run(maxSteps)` 返回 error。

**典型用法**：在跑 Akita cycle-accurate 仿真前，先用 funcsim 跑一遍，看结果是否和预期一致，用来区分是**编译器/映射问题**还是**仿真器/时序问题**。

---

### 3. Report（报告）— `report.go`

**作用**：把 Lint + FuncSim 的结果整理成一份**可读的验证报告**（控制台或文件）。

- 先跑 Lint，再跑 FuncSim
- 报告内容包含：
  - 加载了多少个 PE program
  - Lint：多少 STRUCT / TIMING issue，每条的具体信息
  - FuncSim：是否成功完成、若有错则报错信息
  - 总结：PE 数量、issue 统计、仿真成功/失败
  - **Recommendation**：若有 TIMING 违例，会提示调整 timestep、调度或缓冲策略；若都通过，提示 kernel 可以进仿真

**入口**：`GenerateReport(programs, arch, maxSteps)` 返回 `*VerificationReport`，再调用 `WriteReport(w)` 或 `SaveReportToFile(filename)`。

---

## 和 cycle-accurate simulator 的关系

| 对比项       | verify（Lint + FuncSim）     | core + runtimecfg + config（真实仿真） |
|--------------|------------------------------|----------------------------------------|
| 目的         | 快速验证“对不对”             | 周期精确的“怎么执行、多快”             |
| 时序         | 无周期、无网络延迟           | 有周期、有 backpressure、有延迟        |
| 执行驱动     | 数据依赖就绪即执行           | 引擎 tick、端口收发、调度策略          |
| 速度         | 很快（毫秒级）               | 慢（秒级或更长）                       |
| 典型用法     | 改完 kernel/映射先跑一遍     | 确认无误后再跑完整仿真                 |

**总结**：verify 是仿真前的“守门员”，先保证结构和语义没问题，再上真仿真。

---

## 输入输出小结

- **输入**
  - Kernel YAML（per-PE program，和 `core.LoadProgramFileFromYAML` 一致）
  - 架构参数：`ArchInfo{Rows, Columns, Topology, HopLatency, MemCapacity, CtrlMemItems}`（verify 里自己定义，和 `runtimecfg` 的 arch spec 是分开的）
  - 可选：funcsim 的 `PreloadMemory`、`maxSteps`
- **输出**
  - Lint：`[]Issue`
  - FuncSim：各 PE 的寄存器/内存查询接口 + `Run()` 的 error
  - Report：文本报告（stdout 或文件），包含 Lint 汇总、FuncSim 结果和 Recommendation

---

## 文件结构

| 文件 | 作用 |
|------|------|
| `verify.go` | 类型定义（Issue、ArchInfo、PEState、FunctionalSimulator）、NewFunctionalSimulator、PreloadMemory、GetRegisterValue/GetMemoryValue、GenerateReport 等对外 API |
| `lint.go` | `RunLint`、坐标校验、端口冲突、TIMING 约束检查（含 modulo 支持） |
| `funcsim.go` | `Run`、`canExecuteOp`、`executeOp`、各 opcode 的语义实现（与 core 对齐） |
| `report.go` | `VerificationReport`、`GenerateReport`、`WriteReport`、`SaveReportToFile` |
| `verify_test.go` | 单元测试 |
| `histogram_integration_test*.go` | 用真实 histogram kernel 做集成测试 |
| `cmd/verify-axpy/main.go` 等 | 按 kernel（axpy、histogram、fir、gemv）封装的 CLI，读 YAML + 调 `GenerateReport(...).SaveReportToFile(...)` |

---

## 总结

- **Lint**：静态看“坐标、端口冲突、跨 PE 时序”是否合法。
- **FuncSim**：不跑周期，只按数据流执行，看“算出来的数”对不对。
- **Report**：把前两步结果打成一份报告，并给出是否适合进仿真的建议。

整体上，**verify 包就是在做“不跑完整仿真也能检查 kernel 对不对”的快速验证流水线**。
