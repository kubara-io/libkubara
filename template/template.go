// Package template provides deterministic, hermetic text/template rendering with
// Sprig functions, YAML helpers, output limits, and strict missing-key handling.
//
// By default, the template engine operates hermetically. Functions that read from
// the host environment, query DNS, read clocks, or generate random numbers are disabled.
// This ensures template rendering produces identical output across different environments.
//
// Accessing missing keys in maps causes an immediate error by default ([MissingKeyError]),
// preventing silently omitted configuration values.
package template

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	texttemplate "text/template"

	"github.com/Masterminds/sprig/v3"
	"go.yaml.in/yaml/v3"
)

// MissingKeyMode controls how the template engine behaves when accessing missing map keys.
type MissingKeyMode string

const (
	// MissingKeyError stops execution immediately with an error when a missing key is accessed.
	MissingKeyError MissingKeyMode = "error"
	// MissingKeyZero sets the missing value to the zero value of its type.
	MissingKeyZero MissingKeyMode = "zero"
	// MissingKeyDefault outputs "<no value>" for missing map keys.
	MissingKeyDefault MissingKeyMode = "default"
)

// SprigMode configures which Sprig functions are exposed to templates.
type SprigMode int

const (
	// SprigHermetic excludes non-deterministic functions (environment, clock, random, and network).
	SprigHermetic SprigMode = iota
	// SprigFull exposes the complete Sprig text function map.
	SprigFull
	// SprigDisabled exposes no Sprig functions.
	SprigDisabled
)

// Input specifies the name and source content of a template to render.
type Input struct {
	// Name identifies the template for error reporting and debugging.
	Name string
	// Source contains the raw template text.
	Source []byte
}

// Engine executes text/template inputs with configured delimiters, functions, and safety bounds.
type Engine struct {
	funcs      texttemplate.FuncMap
	missingKey MissingKeyMode
	sprig      SprigMode
	leftDelim  string
	rightDelim string
	maxOutput  int64
	yamlFuncs  bool
}

type engineConfig struct {
	funcs      texttemplate.FuncMap
	missingKey MissingKeyMode
	sprig      SprigMode
	leftDelim  string
	rightDelim string
	maxOutput  int64
	yamlFuncs  bool
}

// Option configures an Engine instance.
type Option func(*engineConfig)

// WithFuncs registers custom template functions.
func WithFuncs(funcs texttemplate.FuncMap) Option {
	return func(c *engineConfig) {
		for name, fn := range funcs {
			c.funcs[name] = fn
		}
	}
}

// WithHermeticSprig enables Sprig functions excluding environment, clock, random, and network functions.
func WithHermeticSprig() Option { return func(c *engineConfig) { c.sprig = SprigHermetic } }

// WithFullSprig enables all standard Sprig text functions.
func WithFullSprig() Option { return func(c *engineConfig) { c.sprig = SprigFull } }

// WithoutSprig disables all Sprig functions.
func WithoutSprig() Option { return func(c *engineConfig) { c.sprig = SprigDisabled } }

// WithoutYAMLFunctions disables the built-in toYaml and fromYaml template functions.
func WithoutYAMLFunctions() Option {
	return func(c *engineConfig) { c.yamlFuncs = false }
}

// WithMissingKey configures the behavior when referencing an absent map key.
func WithMissingKey(mode MissingKeyMode) Option {
	return func(c *engineConfig) { c.missingKey = mode }
}

// WithMissingKeyError configures the engine to fail if a template accesses an absent map key.
func WithMissingKeyError() Option { return WithMissingKey(MissingKeyError) }

// WithDelims sets custom left and right template delimiters (e.g. "[[" and "]]").
func WithDelims(left, right string) Option {
	return func(c *engineConfig) {
		c.leftDelim = left
		c.rightDelim = right
	}
}

// WithMaxOutput sets the maximum rendered output byte limit per template execution.
func WithMaxOutput(bytes int64) Option { return func(c *engineConfig) { c.maxOutput = bytes } }

var functionName = regexp.MustCompile(`^[[:alnum:]_]+$`)

// New creates a new Engine with the provided configuration options.
func New(options ...Option) (*Engine, error) {
	cfg := engineConfig{
		funcs:      texttemplate.FuncMap{},
		missingKey: MissingKeyError,
		sprig:      SprigHermetic,
		maxOutput:  16 << 20,
		yamlFuncs:  true,
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if cfg.missingKey != MissingKeyError && cfg.missingKey != MissingKeyZero && cfg.missingKey != MissingKeyDefault {
		return nil, fmt.Errorf("invalid missing-key mode %q", cfg.missingKey)
	}
	if (cfg.leftDelim == "") != (cfg.rightDelim == "") {
		return nil, fmt.Errorf("both template delimiters must be set")
	}
	for name, fn := range cfg.funcs {
		if !functionName.MatchString(name) {
			return nil, fmt.Errorf("invalid template function name %q", name)
		}
		if fn == nil || reflect.TypeOf(fn).Kind() != reflect.Func {
			return nil, fmt.Errorf("template function %q is not a function", name)
		}
	}

	return &Engine{
		funcs:      cloneFuncMap(cfg.funcs),
		missingKey: cfg.missingKey,
		sprig:      cfg.sprig,
		leftDelim:  cfg.leftDelim,
		rightDelim: cfg.rightDelim,
		maxOutput:  cfg.maxOutput,
		yamlFuncs:  cfg.yamlFuncs,
	}, nil
}

func cloneFuncMap(in texttemplate.FuncMap) texttemplate.FuncMap {
	out := make(texttemplate.FuncMap, len(in))
	for name, fn := range in {
		out[name] = fn
	}
	return out
}

func (e *Engine) funcMap() texttemplate.FuncMap {
	funcs := texttemplate.FuncMap{}
	switch e.sprig {
	case SprigHermetic:
		for name, fn := range sprig.HermeticTxtFuncMap() {
			funcs[name] = fn
		}
	case SprigFull:
		for name, fn := range sprig.TxtFuncMap() {
			funcs[name] = fn
		}
	}
	if e.yamlFuncs {
		funcs["toYaml"] = toYAML
		funcs["fromYaml"] = fromYAML
	}
	for name, fn := range e.funcs {
		funcs[name] = fn
	}
	return funcs
}

func toYAML(value any) (string, error) {
	out, err := yaml.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal YAML: %w", err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func fromYAML(value string) (any, error) {
	var out any
	if err := yaml.Unmarshal([]byte(value), &out); err != nil {
		return nil, fmt.Errorf("unmarshal YAML: %w", err)
	}
	return out, nil
}

// Render parses and executes a template against data, returning the rendered output.
//
// Example:
//
//	engine, err := template.New(template.WithMissingKeyError())
//	if err != nil {
//		return err
//	}
//	output, err := engine.Render(ctx, template.Input{
//		Name:   "deployment.yaml",
//		Source: []byte("replicas: {{ .config.spec.replicas }}"),
//	}, contextData)
//
// It returns an error if ctx is canceled, if template syntax is invalid, if a
// missing map key is referenced under [MissingKeyError], or if output exceeds [WithMaxOutput].
func (e *Engine) Render(ctx context.Context, input Input, data any) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("template engine is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := input.Name
	if name == "" {
		name = "template"
	}

	tmpl := texttemplate.New(name).Funcs(e.funcMap()).Option("missingkey=" + string(e.missingKey))
	if e.leftDelim != "" {
		tmpl = tmpl.Delims(e.leftDelim, e.rightDelim)
	}
	parsed, err := tmpl.Parse(string(input.Source))
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", name, err)
	}

	var buffer bytes.Buffer
	writer := &contextWriter{ctx: ctx, dst: &buffer, limited: e.maxOutput > 0, remaining: e.maxOutput}
	if err := parsed.Execute(writer, data); err != nil {
		return nil, fmt.Errorf("execute template %q: %w", name, err)
	}
	return buffer.Bytes(), nil
}

// ErrOutputLimit is returned when template rendering exceeds the configured maximum output limit.
var ErrOutputLimit = errors.New("template output limit exceeded")

type contextWriter struct {
	ctx       context.Context
	dst       *bytes.Buffer
	limited   bool
	remaining int64
}

func (w *contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if w.limited && int64(len(p)) > w.remaining {
		return 0, ErrOutputLimit
	}
	n, err := w.dst.Write(p)
	if w.limited {
		w.remaining -= int64(n)
	}
	return n, err
}
