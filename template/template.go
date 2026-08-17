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

type MissingKeyMode string

const (
	MissingKeyError   MissingKeyMode = "error"
	MissingKeyZero    MissingKeyMode = "zero"
	MissingKeyDefault MissingKeyMode = "default"
)

type SprigMode int

const (
	// SprigHermetic excludes environment, clock, random, and network functions.
	SprigHermetic SprigMode = iota
	// SprigFull exposes the complete Sprig text function map.
	SprigFull
	// SprigDisabled exposes no Sprig functions.
	SprigDisabled
)

type Input struct {
	Name   string
	Source []byte
}

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

type Option func(*engineConfig)

func WithFuncs(funcs texttemplate.FuncMap) Option {
	return func(c *engineConfig) {
		for name, fn := range funcs {
			c.funcs[name] = fn
		}
	}
}

func WithHermeticSprig() Option { return func(c *engineConfig) { c.sprig = SprigHermetic } }
func WithFullSprig() Option     { return func(c *engineConfig) { c.sprig = SprigFull } }
func WithoutSprig() Option      { return func(c *engineConfig) { c.sprig = SprigDisabled } }
func WithoutYAMLFunctions() Option {
	return func(c *engineConfig) { c.yamlFuncs = false }
}
func WithMissingKey(mode MissingKeyMode) Option {
	return func(c *engineConfig) { c.missingKey = mode }
}
func WithMissingKeyError() Option { return WithMissingKey(MissingKeyError) }
func WithDelims(left, right string) Option {
	return func(c *engineConfig) {
		c.leftDelim = left
		c.rightDelim = right
	}
}

func WithMaxOutput(bytes int64) Option { return func(c *engineConfig) { c.maxOutput = bytes } }

var functionName = regexp.MustCompile(`^[[:alnum:]_]+$`)

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
