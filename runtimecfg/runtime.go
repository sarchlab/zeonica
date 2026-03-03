package runtimecfg

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
)

const (
	defaultRows            = 4
	defaultColumns         = 4
	defaultExecutionModel  = "serial"
	defaultExecutionPolicy = "in_order_dataflow"
	defaultDriverName      = "Driver"
	defaultDeviceName      = "Device"
	defaultLogTemplate     = "<test>.json.log"
)

var freqPattern = regexp.MustCompile(`^([0-9]+)\s*(ghz|mhz|khz|hz)$`)

// ResolvedConfig is the executable runtime configuration after defaults/resolution.
type ResolvedConfig struct {
	TestName           string
	Rows               int
	Columns            int
	ExecutionModel     string
	ExecutionPolicy    string
	DriverName         string
	DriverFreq         sim.Freq
	DeviceName         string
	DeviceFreq         sim.Freq
	BindToArchitecture bool
	LoggingEnabled     bool
	LogPath            string
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
}

// LoadRuntime loads arch spec, resolves config, and builds runtime objects.
func LoadRuntime(specPath, testName string) (*Runtime, error) {
	spec, err := Load(specPath)
	if err != nil {
		return nil, err
	}

	cfg, err := Resolve(spec, testName)
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
func Resolve(spec ArchSpec, testName string) (ResolvedConfig, error) {
	resolved := ResolvedConfig{
		TestName:           normalizeTestName(testName),
		Rows:               defaultOrPositive(spec.CGRADefaults.Rows, defaultRows),
		Columns:            defaultOrPositive(spec.CGRADefaults.Columns, defaultColumns),
		ExecutionModel:     defaultOrString(spec.Simulator.ExecutionModel, defaultExecutionModel),
		ExecutionPolicy:    defaultOrString(spec.Simulator.ExecutionPolicy, defaultExecutionPolicy),
		DriverName:         defaultOrString(spec.Simulator.Driver.Name, defaultDriverName),
		DeviceName:         defaultOrString(spec.Simulator.Device.Name, defaultDeviceName),
		BindToArchitecture: defaultOrBool(spec.Simulator.Device.BindToArchitecture, true),
		LoggingEnabled:     defaultOrBool(spec.Simulator.Logging.Enabled, true),
	}

	normalizedPolicy, err := normalizeExecutionPolicy(resolved.ExecutionPolicy)
	if err != nil {
		return ResolvedConfig{}, err
	}
	resolved.ExecutionPolicy = normalizedPolicy

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

	return resolved, nil
}

// BuildRuntime builds engine, driver, and device from a resolved config.
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
		Build(cfg.DriverName)

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(cfg.DeviceFreq).
		WithWidth(width).
		WithHeight(height).
		WithExecutionPolicy(cfg.ExecutionPolicy).
		Build(cfg.DeviceName)

	driver.RegisterDevice(device)

	return &Runtime{
		Config: cfg,
		Engine: engine,
		Driver: driver,
		Device: device,
	}, nil
}

// InitTraceLogger initializes the default slog JSON trace logger.
func (r *Runtime) InitTraceLogger(level slog.Leveler) (*os.File, error) {
	if !r.Config.LoggingEnabled {
		return nil, nil
	}

	file, err := os.Create(r.Config.LogPath)
	if err != nil {
		return nil, fmt.Errorf("create trace log file: %w", err)
	}

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
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
