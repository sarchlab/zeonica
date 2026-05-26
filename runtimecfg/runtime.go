package runtimecfg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/report"
)

const (
	defaultRows               = 4
	defaultColumns            = 4
	defaultExecutionModel     = "serial"
	defaultExecutionPolicy    = "in_order_dataflow"
	defaultStrictMaxSlip      = int64(4)
	defaultStrictFail         = false
	defaultEnableFIFOModel    = false
	defaultEnableQueueWatches = false
	defaultDriverName         = "Driver"
	defaultDeviceName         = "Device"
	defaultLogTemplate        = "<test>.json.log"

	defaultDriverPortIncomingBufferDepth = 1
	defaultDriverPortOutgoingBufferDepth = 1
	defaultCorePortIncomingBufferDepth   = 1
	defaultCorePortOutgoingBufferDepth   = 1
	defaultNumRegisters                  = 64
	defaultLocalMemoryWords              = 1024
	defaultEnableVectorPE                = false
	defaultVectorLanes                   = 1
	defaultMemoryMode                    = "simple"
	defaultSharedMemoryModel             = "ideal"
	defaultSharedMemoryBanks             = 1
	defaultSharedMemoryBaseLatency       = 5
	defaultSharedMemoryBankInterleave    = 4
	defaultLinkLatency                   = 1
	defaultLinkBandwidth                 = 32
	linkTimingModelParseOnly             = "parse_only"
)

var freqPattern = regexp.MustCompile(`^([0-9]+)\s*(ghz|mhz|khz|hz)$`)

// ResolvedConfig is the executable runtime configuration after defaults/resolution.
type ResolvedConfig struct {
	TestName              string
	Rows                  int
	Columns               int
	ExecutionModel        string
	ExecutionPolicy       string
	StrictMaxSlip         int64
	StrictFailOnViolation bool
	EnableFIFOModel       bool
	EnableQueueWatches    bool
	DriverName            string
	DriverFreq            sim.Freq
	DeviceName            string
	DeviceFreq            sim.Freq
	BindToArchitecture    bool
	LoggingEnabled        bool
	EnableTrace           bool
	LogPath               string

	DriverPortIncomingBufferDepth int
	DriverPortOutgoingBufferDepth int
	CorePortIncomingBufferDepth   int
	CorePortOutgoingBufferDepth   int
	NumRegisters                  int
	LocalMemoryWords              int
	EnableVectorPE                bool
	VectorLanes                   int
	MemoryMode                    string
	MemoryShare                   map[[2]int]int
	MemoryShareBase               map[[2]int]uint32
	SharedMemoryModel             string
	SharedMemoryBanks             int
	SharedMemoryBaseLatency       int
	SharedMemoryBankInterleave    int
	LinkLatency                   int
	LinkBandwidth                 int
	LinkTimingModel               string
	ProgramYAML                   string
	ReportName                    string
	QueueWatches                  []core.QueueWatchSpec
	BufferSweepDepths             []int
}

// BuildOverrides allows optional size override when not binding to architecture.
type BuildOverrides struct {
	Width  int
	Height int
}

// Runtime holds initialized simulator objects and resolved configuration.
type Runtime struct {
	Spec     ArchSpec
	SpecPath string
	Config   ResolvedConfig
	Engine   sim.Engine
	Driver   api.Driver
	Device   cgra.Device
	Observer *report.Observer
}

// LoadRuntime loads arch spec, resolves config, and builds runtime objects.
func LoadRuntime(specPath, testName string) (*Runtime, error) {
	spec, err := Load(specPath)
	if err != nil {
		return nil, err
	}

	cfg, err := ResolveWithSpecPath(spec, specPath, testName)
	if err != nil {
		return nil, err
	}

	rt, err := BuildRuntime(cfg, nil)
	if err != nil {
		return nil, err
	}
	rt.Spec = spec
	rt.SpecPath = specPath
	return rt, nil
}

// Resolve resolves defaults and validates runtime values from ArchSpec.
//
//nolint:gocyclo,funlen
func Resolve(spec ArchSpec, testName string) (ResolvedConfig, error) {
	return ResolveWithSpecPath(spec, "", testName)
}

// ResolveWithSpecPath resolves defaults and validates runtime values from ArchSpec,
// using specPath to resolve case2 relative paths when available.
//
//nolint:gocyclo,funlen
func ResolveWithSpecPath(spec ArchSpec, specPath, testName string) (ResolvedConfig, error) {
	programYAML := resolveSpecRelativePath(specPath, spec.Simulator.ProgramYAML)
	reportName := strings.TrimSpace(spec.Simulator.ReportName)
	queueWatches := append([]core.QueueWatchSpec(nil), spec.Simulator.QueueWatches...)
	bufferSweepDepths, err := resolveBufferSweepDepths(spec.Simulator.BufferSweepDepths)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if err := core.ValidateQueueWatchSpecs(queueWatches); err != nil {
		return ResolvedConfig{}, fmt.Errorf("simulator.queue_watches: %w", err)
	}

	effectiveTestName := strings.TrimSpace(testName)
	if effectiveTestName == "" && reportName != "" {
		effectiveTestName = reportName
	}

	resolved := ResolvedConfig{
		TestName:                      normalizeTestName(effectiveTestName),
		Rows:                          defaultOrPositive(spec.CGRADefaults.Rows, defaultRows),
		Columns:                       defaultOrPositive(spec.CGRADefaults.Columns, defaultColumns),
		ExecutionModel:                defaultOrString(spec.Simulator.ExecutionModel, defaultExecutionModel),
		ExecutionPolicy:               defaultOrString(spec.Simulator.ExecutionPolicy, defaultExecutionPolicy),
		EnableFIFOModel:               defaultOrBool(spec.Simulator.EnableFIFOModel, defaultEnableFIFOModel),
		EnableQueueWatches:            defaultOrBool(spec.Simulator.EnableQueueWatches, defaultEnableQueueWatches),
		StrictMaxSlip:                 defaultOrInt64(spec.Simulator.StrictMaxSlip, defaultStrictMaxSlip),
		StrictFailOnViolation:         defaultOrBool(spec.Simulator.StrictFailOnViolation, defaultStrictFail),
		DriverName:                    defaultOrString(spec.Simulator.Driver.Name, defaultDriverName),
		DeviceName:                    defaultOrString(spec.Simulator.Device.Name, defaultDeviceName),
		BindToArchitecture:            defaultOrBool(spec.Simulator.Device.BindToArchitecture, true),
		LoggingEnabled:                defaultOrBool(spec.Simulator.Logging.Enabled, true),
		EnableTrace:                   defaultOrBool(spec.Simulator.Logging.EnableTrace, false),
		LinkTimingModel:               linkTimingModelParseOnly,
		DriverPortIncomingBufferDepth: defaultDriverPortIncomingBufferDepth,
		DriverPortOutgoingBufferDepth: defaultDriverPortOutgoingBufferDepth,
		CorePortIncomingBufferDepth:   defaultCorePortIncomingBufferDepth,
		CorePortOutgoingBufferDepth:   defaultCorePortOutgoingBufferDepth,
		NumRegisters:                  defaultNumRegisters,
		LocalMemoryWords:              defaultLocalMemoryWords,
		EnableVectorPE:                defaultOrBool(spec.Simulator.Device.EnableVectorPE, defaultEnableVectorPE),
		VectorLanes:                   defaultVectorLanes,
		MemoryMode:                    defaultMemoryMode,
		SharedMemoryModel:             defaultSharedMemoryModel,
		SharedMemoryBanks:             defaultSharedMemoryBanks,
		SharedMemoryBaseLatency:       defaultSharedMemoryBaseLatency,
		SharedMemoryBankInterleave:    defaultSharedMemoryBankInterleave,
		LinkLatency:                   defaultLinkLatency,
		LinkBandwidth:                 defaultLinkBandwidth,
		ProgramYAML:                   programYAML,
		ReportName:                    reportName,
		QueueWatches:                  queueWatches,
		BufferSweepDepths:             bufferSweepDepths,
	}

	normalizedPolicy, err := normalizeExecutionPolicy(resolved.ExecutionPolicy)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.ExecutionPolicy = normalizedPolicy

	if envSlip, ok, err := parseInt64Env("ZEONICA_STRICT_MAX_SLIP"); err != nil {
		return ResolvedConfig{}, err
	} else if ok {
		resolved.StrictMaxSlip = envSlip
	}
	if envFail, ok, err := parseBoolEnv("ZEONICA_STRICT_FAIL_ON_VIOLATION"); err != nil {
		return ResolvedConfig{}, err
	} else if ok {
		resolved.StrictFailOnViolation = envFail
	}

	driverFreq, err := parseFrequency(spec.Simulator.Driver.Frequency, 1*sim.GHz)
	if err != nil {
		return ResolvedConfig{}, fmt.Errorf("resolve driver frequency: %w", err)
	}
	resolved.DriverFreq = driverFreq

	deviceFreq, err := parseFrequency(spec.Simulator.Device.Frequency, 1*sim.GHz)
	if err != nil {
		return ResolvedConfig{}, fmt.Errorf("resolve device frequency: %w", err)
	}
	resolved.DeviceFreq = deviceFreq

	logTemplate := defaultOrString(spec.Simulator.Logging.File, defaultLogTemplate)
	resolved.LogPath = resolveLogPath(logTemplate, resolved.TestName)

	resolved.DriverPortIncomingBufferDepth, err = resolvePositivePtr(
		spec.Simulator.Driver.PortIncomingBufferDepth,
		defaultDriverPortIncomingBufferDepth,
		"simulator.driver.port_incoming_buffer_depth",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.DriverPortOutgoingBufferDepth, err = resolvePositivePtr(
		spec.Simulator.Driver.PortOutgoingBufferDepth,
		defaultDriverPortOutgoingBufferDepth,
		"simulator.driver.port_outgoing_buffer_depth",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.CorePortIncomingBufferDepth, err = resolvePositivePtr(
		spec.Simulator.Device.PortIncomingBufferDepth,
		defaultCorePortIncomingBufferDepth,
		"simulator.device.port_incoming_buffer_depth",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.CorePortOutgoingBufferDepth, err = resolvePositivePtr(
		spec.Simulator.Device.PortOutgoingBufferDepth,
		defaultCorePortOutgoingBufferDepth,
		"simulator.device.port_outgoing_buffer_depth",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}

	resolved.NumRegisters, err = resolvePositive(
		spec.TileDefaults.NumRegisters,
		defaultNumRegisters,
		"tile_defaults.num_registers",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.LocalMemoryWords, err = resolvePositive(
		spec.TileDefaults.LocalMemoryWords,
		defaultLocalMemoryWords,
		"tile_defaults.local_memory_words",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if resolved.EnableVectorPE {
		resolved.VectorLanes, err = resolvePositive(
			spec.TileDefaults.VectorLanes,
			defaultVectorLanes,
			"tile_defaults.vector_lanes",
		)
		if err != nil {
			return ResolvedConfig{}, err
		}
	} else {
		resolved.VectorLanes = 1
	}

	resolved.MemoryMode, err = normalizeMemoryMode(defaultOrString(spec.Simulator.Device.MemoryMode, defaultMemoryMode))
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.MemoryShare, err = resolveMemoryShare(
		resolved.MemoryMode,
		resolved.Rows,
		resolved.Columns,
		spec.Simulator.Device.MemoryShare,
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.MemoryShareBase, err = resolveMemoryShareBase(
		resolved.MemoryMode,
		resolved.Rows,
		resolved.Columns,
		spec.Simulator.Device.MemoryShare,
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.SharedMemoryModel, err = normalizeSharedMemoryModel(defaultOrString(
		spec.Simulator.Device.SharedMemoryModel,
		defaultSharedMemoryModel,
	))
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.SharedMemoryBanks, err = resolvePositive(
		spec.Simulator.Device.SharedMemoryBanks,
		defaultSharedMemoryBanks,
		"simulator.device.shared_memory_banks",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.SharedMemoryBaseLatency, err = resolvePositive(
		spec.Simulator.Device.SharedMemoryBaseLatency,
		defaultSharedMemoryBaseLatency,
		"simulator.device.shared_memory_base_latency",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.SharedMemoryBankInterleave, err = resolvePositive(
		spec.Simulator.Device.SharedMemoryInterleave,
		defaultSharedMemoryBankInterleave,
		"simulator.device.shared_memory_bank_interleave_bytes",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}

	resolved.LinkLatency, err = resolveNonNegativePtr(
		spec.LinkDefaults.Latency,
		defaultLinkLatency,
		"link_defaults.latency",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.LinkBandwidth, err = resolvePositivePtr(
		spec.LinkDefaults.Bandwidth,
		defaultLinkBandwidth,
		"link_defaults.bandwidth",
	)
	if err != nil {
		return ResolvedConfig{}, err
	}

	return resolved, nil
}

// BuildRuntime builds engine, driver, and device from a resolved config.
//
//nolint:funlen
func BuildRuntime(cfg ResolvedConfig, overrides *BuildOverrides) (*Runtime, error) {
	executionModel := strings.ToLower(strings.TrimSpace(cfg.ExecutionModel))
	var engine sim.Engine
	switch executionModel {
	case "", "serial":
		engine = sim.NewSerialEngine()
	default:
		return nil, fmt.Errorf("unsupported execution_model %q (currently only serial is supported)", cfg.ExecutionModel)
	}

	width := cfg.Columns
	height := cfg.Rows
	if !cfg.BindToArchitecture && overrides != nil {
		if overrides.Width > 0 {
			width = overrides.Width
		}
		if overrides.Height > 0 {
			height = overrides.Height
		}
	}

	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(cfg.DriverFreq).
		WithPortBufferDepth(cfg.DriverPortIncomingBufferDepth, cfg.DriverPortOutgoingBufferDepth).
		Build(cfg.DriverName)

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(cfg.DeviceFreq).
		WithWidth(width).
		WithHeight(height).
		WithExecutionPolicy(cfg.ExecutionPolicy).
		WithStrictTimingConfig(cfg.StrictMaxSlip, cfg.StrictFailOnViolation).
		WithMemoryMode(cfg.MemoryMode).
		WithMemoryShare(cfg.MemoryShare).
		WithSharedMemoryBase(cfg.MemoryShareBase).
		WithSharedMemoryModel(cfg.SharedMemoryModel).
		WithSharedMemoryBankConfig(
			cfg.SharedMemoryBanks,
			cfg.SharedMemoryBaseLatency,
			uint64(cfg.SharedMemoryBankInterleave),
		).
		WithCorePortBufferDepth(cfg.CorePortIncomingBufferDepth, cfg.CorePortOutgoingBufferDepth).
		WithEnableFIFOModel(cfg.EnableFIFOModel).
		WithEnableQueueWatches(cfg.EnableQueueWatches).
		WithQueueWatches(cfg.QueueWatches).
		WithRegisterCount(cfg.NumRegisters).
		WithLocalMemoryWords(cfg.LocalMemoryWords).
		WithVectorConfig(cfg.EnableVectorPE, cfg.VectorLanes).
		Build(cfg.DeviceName)

	if cfg.LinkTimingModel == linkTimingModelParseOnly {
		slog.Info(
			"link_defaults parsed in parse-only mode",
			"latency", cfg.LinkLatency,
			"bandwidth", cfg.LinkBandwidth,
		)
	}

	driver.RegisterDevice(device)

	return &Runtime{
		Config:   cfg,
		Engine:   engine,
		Driver:   driver,
		Device:   device,
		Observer: report.NewObserver(),
	}, nil
}

func resolveSpecRelativePath(specPath, target string) string {
	trimmedTarget := strings.TrimSpace(target)
	if trimmedTarget == "" {
		return ""
	}
	cleanTarget := filepath.Clean(trimmedTarget)
	if filepath.IsAbs(cleanTarget) || strings.TrimSpace(specPath) == "" {
		return cleanTarget
	}
	return filepath.Clean(filepath.Join(filepath.Dir(specPath), cleanTarget))
}

func resolveBufferSweepDepths(input []int) ([]int, error) {
	if len(input) == 0 {
		return nil, nil
	}
	depths := make([]int, 0, len(input))
	for idx, depth := range input {
		if depth <= 0 {
			return nil, fmt.Errorf("simulator.buffer_sweep_depths[%d] must be > 0", idx)
		}
		depths = append(depths, depth)
	}
	return depths, nil
}

// InitTraceLogger initializes the default slog JSON trace logger.
func (r *Runtime) InitTraceLogger(level slog.Leveler) (*os.File, error) {
	file, err := os.Create(r.Config.LogPath)
	if err != nil {
		return nil, fmt.Errorf("create trace log file: %w", err)
	}

	core.SetTraceObserver(nil)
	if r.Observer != nil {
		core.SetTraceObserver(r.Observer.Observe)
	}
	core.SetTraceEnabled(r.Config.EnableTrace)

	if !r.Config.LoggingEnabled || !r.Config.EnableTrace {
		stdoutHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelError,
		})
		slog.SetDefault(slog.New(stdoutHandler))
		return file, nil
	}

	traceHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: level,
	})
	stdoutHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	})
	slog.SetDefault(slog.New(newTeeHandler(stdoutHandler, traceHandler)))
	return file, nil
}

// CloseTraceLog flushes and closes the trace log file.
func CloseTraceLog(file *os.File) error {
	if file == nil {
		return nil
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync trace log: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close trace log: %w", err)
	}
	return nil
}

func defaultOrPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultOrString(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func defaultOrBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func defaultOrInt64(value *int64, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeTestName(testName string) string {
	trimmed := strings.TrimSpace(testName)
	if trimmed == "" {
		return "test"
	}
	return trimmed
}

func resolveLogPath(template, testName string) string {
	resolved := strings.ReplaceAll(template, "<test>", testName)
	if strings.TrimSpace(resolved) == "" {
		return strings.ReplaceAll(defaultLogTemplate, "<test>", testName)
	}
	return resolved
}

func parseFrequency(input string, fallback sim.Freq) (sim.Freq, error) {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return fallback, nil
	}

	matches := freqPattern.FindStringSubmatch(text)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid frequency format %q, expected like 1GHz/500MHz", input)
	}

	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse frequency value: %w", err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("frequency must be positive")
	}

	switch matches[2] {
	case "ghz":
		return sim.Freq(value) * sim.GHz, nil
	case "mhz":
		return sim.Freq(value) * sim.MHz, nil
	case "khz":
		return sim.Freq(value) * sim.KHz, nil
	case "hz":
		return sim.Freq(value), nil
	default:
		return 0, fmt.Errorf("unsupported frequency unit %q", matches[2])
	}
}

func parseInt64Env(name string) (int64, bool, error) {
	raw, exists := os.LookupEnv(name)
	if !exists {
		return 0, false, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false, nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid %s=%q: %w", name, raw, err)
	}
	return value, true, nil
}

func parseBoolEnv(name string) (bool, bool, error) {
	raw, exists := os.LookupEnv(name)
	if !exists {
		return false, false, nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, false, nil
	}
	value, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, false, fmt.Errorf("invalid %s=%q: %w", name, raw, err)
	}
	return value, true, nil
}

func normalizeExecutionPolicy(input string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(input))
	switch text {
	case "", "in_order_dataflow", "in-order-dataflow", "dynamic":
		return "in_order_dataflow", nil
	case "elastic_scheduled", "elastic-scheduled", "hybrid":
		return "elastic_scheduled", nil
	case "strict_timed", "strict-timed", "static":
		return "strict_timed", nil
	default:
		return "", fmt.Errorf(
			"unsupported execution_policy %q (supported: strict_timed, elastic_scheduled, in_order_dataflow)",
			input,
		)
	}
}

func normalizeMemoryMode(input string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(input))
	switch text {
	case "", "simple":
		return "simple", nil
	case "shared":
		return "shared", nil
	case "local":
		return "local", nil
	default:
		return "", fmt.Errorf("unsupported memory_mode %q (supported: simple, shared, local)", input)
	}
}

func normalizeSharedMemoryModel(input string) (string, error) {
	text := strings.ToLower(strings.TrimSpace(input))
	switch text {
	case "", "ideal":
		return "ideal", nil
	case "banked":
		return "banked", nil
	default:
		return "", fmt.Errorf("unsupported shared_memory_model %q (supported: ideal, banked)", input)
	}
}

func resolvePositive(value, fallback int, field string) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be > 0, got %d", field, value)
	}
	return value, nil
}

func resolvePositivePtr(value *int, fallback int, field string) (int, error) {
	if value == nil {
		return fallback, nil
	}
	if *value <= 0 {
		return 0, fmt.Errorf("%s must be > 0, got %d", field, *value)
	}
	return *value, nil
}

func resolveNonNegativePtr(value *int, fallback int, field string) (int, error) {
	if value == nil {
		return fallback, nil
	}
	if *value < 0 {
		return 0, fmt.Errorf("%s must be >= 0, got %d", field, *value)
	}
	return *value, nil
}

func resolveMemoryShare(mode string, rows, cols int, entries []MemoryShareEntry) (map[[2]int]int, error) {
	if mode != "shared" {
		return nil, nil
	}

	if len(entries) == 0 {
		return defaultMemoryShare(rows, cols), nil
	}

	share := make(map[[2]int]int, len(entries))
	for _, entry := range entries {
		if entry.TileX < 0 || entry.TileX >= cols || entry.TileY < 0 || entry.TileY >= rows {
			return nil, fmt.Errorf(
				"simulator.device.memory_share has out-of-range tile (%d,%d) for grid %dx%d",
				entry.TileX,
				entry.TileY,
				cols,
				rows,
			)
		}
		if entry.Group < 0 {
			return nil, fmt.Errorf("simulator.device.memory_share group must be >= 0, got %d", entry.Group)
		}
		share[[2]int{entry.TileX, entry.TileY}] = entry.Group
	}
	return share, nil
}

func defaultMemoryShare(rows, cols int) map[[2]int]int {
	share := make(map[[2]int]int, rows*cols)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			share[[2]int{x, y}] = 0
		}
	}
	return share
}

func resolveMemoryShareBase(mode string, rows, cols int, entries []MemoryShareEntry) (map[[2]int]uint32, error) {
	if mode != "shared" {
		return nil, nil
	}
	bases := make(map[[2]int]uint32, len(entries))
	for _, entry := range entries {
		if entry.TileX < 0 || entry.TileX >= cols || entry.TileY < 0 || entry.TileY >= rows {
			return nil, fmt.Errorf(
				"simulator.device.memory_share has out-of-range tile (%d,%d) for grid %dx%d",
				entry.TileX,
				entry.TileY,
				cols,
				rows,
			)
		}
		bases[[2]int{entry.TileX, entry.TileY}] = entry.Base
	}
	return bases, nil
}

type teeHandler struct {
	handlers []slog.Handler
}

func newTeeHandler(handlers ...slog.Handler) slog.Handler {
	cleaned := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			cleaned = append(cleaned, handler)
		}
	}
	return &teeHandler{handlers: cleaned}
}

func (h *teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *teeHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithAttrs(attrs))
	}
	return &teeHandler{handlers: next}
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithGroup(name))
	}
	return &teeHandler{handlers: next}
}
