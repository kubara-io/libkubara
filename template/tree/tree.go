package tree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	libtemplate "github.com/kubara-io/libkubara/template"
)

type Source struct {
	Name  string
	FS    fs.FS
	Roots []string
}

type Entry struct {
	Source      string
	SourceIndex int
	Path        string
	FS          fs.FS
}

type Matcher func(Entry) bool
type Predicate func(Entry) bool
type PathFunc func(Entry) (string, error)
type KeyFunc func(Entry) (string, error)
type CollisionResolver func(current, next Entry) (replaceCurrent bool, err error)

type CollisionPolicy int

const (
	CollisionError CollisionPolicy = iota
	CollisionPreferFirst
	CollisionPreferLast
)

type Result struct {
	Source    string
	InputPath string
	Path      string
	Content   []byte
	Error     error
}

type config struct {
	sources   []Source
	matcher   Matcher
	predicate Predicate
	path      PathFunc
	key       KeyFunc
	collision CollisionPolicy
	resolver  CollisionResolver
}

type Option func(*config)

func WithSources(sources ...Source) Option {
	return func(c *config) { c.sources = append(c.sources, sources...) }
}
func WithTemplateMatcher(matcher Matcher) Option { return func(c *config) { c.matcher = matcher } }
func WithPredicate(predicate Predicate) Option   { return func(c *config) { c.predicate = predicate } }
func WithPathFunc(fn PathFunc) Option            { return func(c *config) { c.path = fn } }
func WithKeyFunc(fn KeyFunc) Option              { return func(c *config) { c.key = fn } }
func WithCollisionPolicy(policy CollisionPolicy) Option {
	return func(c *config) { c.collision = policy }
}
func WithCollisionResolver(resolver CollisionResolver) Option {
	return func(c *config) { c.resolver = resolver }
}

func Suffix(suffix string) Matcher {
	return func(entry Entry) bool { return strings.HasSuffix(entry.Path, suffix) }
}

type Renderer struct {
	engine *libtemplate.Engine
	config config
}

func New(engine *libtemplate.Engine, options ...Option) (*Renderer, error) {
	if engine == nil {
		return nil, fmt.Errorf("template engine is required")
	}
	cfg := config{
		matcher:   func(entry Entry) bool { return strings.HasSuffix(entry.Path, ".tplt") },
		predicate: func(Entry) bool { return true },
		path:      func(entry Entry) (string, error) { return strings.TrimSuffix(entry.Path, ".tplt"), nil },
		key:       func(entry Entry) (string, error) { return entry.Path, nil },
		collision: CollisionError,
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if len(cfg.sources) == 0 {
		return nil, fmt.Errorf("at least one tree source is required")
	}
	for i, source := range cfg.sources {
		if source.FS == nil {
			return nil, fmt.Errorf("source %d has a nil filesystem", i)
		}
		if source.Name == "" {
			cfg.sources[i].Name = fmt.Sprintf("source-%d", i)
		}
		if len(source.Roots) == 0 {
			cfg.sources[i].Roots = []string{"."}
		}
	}
	return &Renderer{engine: engine, config: cfg}, nil
}

func (r *Renderer) discover() ([]Entry, error) {
	var entries []Entry
	for index, source := range r.config.sources {
		for _, root := range source.Roots {
			err := fs.WalkDir(source.FS, root, func(filePath string, item fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if item.IsDir() {
					return nil
				}
				entry := Entry{Source: source.Name, SourceIndex: index, Path: path.Clean(filePath), FS: source.FS}
				if r.config.predicate(entry) {
					entries = append(entries, entry)
				}
				return nil
			})
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("walk source %q root %q: %w", source.Name, root, err)
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].SourceIndex != entries[j].SourceIndex {
			return entries[i].SourceIndex < entries[j].SourceIndex
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func (r *Renderer) Render(ctx context.Context, data any) ([]Result, error) {
	return r.render(ctx, data, false)
}

// RenderAll returns every selected result. Template and file-read errors are
// retained on their result and joined in the returned error.
func (r *Renderer) RenderAll(ctx context.Context, data any) ([]Result, error) {
	return r.render(ctx, data, true)
}

func (r *Renderer) render(ctx context.Context, data any, continueOnError bool) ([]Result, error) {
	entries, err := r.discover()
	if err != nil {
		return nil, err
	}

	selected := map[string]Entry{}
	for _, entry := range entries {
		key, err := r.config.key(entry)
		if err != nil {
			return nil, fmt.Errorf("resolve key for %q: %w", entry.Path, err)
		}
		previous, exists := selected[key]
		if exists {
			if r.config.resolver != nil {
				replaceCurrent, err := r.config.resolver(previous, entry)
				if err != nil {
					return nil, fmt.Errorf("resolve collision for tree key %q: %w", key, err)
				}
				if !replaceCurrent {
					continue
				}
				selected[key] = entry
				continue
			}
			switch r.config.collision {
			case CollisionError:
				return nil, fmt.Errorf("tree key %q exists in both %q and %q", key, previous.Source, entry.Source)
			case CollisionPreferFirst:
				continue
			case CollisionPreferLast:
			}
		}
		selected[key] = entry
	}

	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	results := make([]Result, 0, len(keys))
	var renderErrors []error
	for _, key := range keys {
		entry := selected[key]
		content, err := fs.ReadFile(entry.FS, entry.Path)
		if err != nil {
			if continueOnError {
				results = append(results, Result{Source: entry.Source, InputPath: entry.Path, Error: err})
				renderErrors = append(renderErrors, fmt.Errorf("read %q from %q: %w", entry.Path, entry.Source, err))
				continue
			}
			return nil, fmt.Errorf("read %q from %q: %w", entry.Path, entry.Source, err)
		}
		outputPath, err := r.config.path(entry)
		if err != nil {
			return nil, fmt.Errorf("resolve output path for %q: %w", entry.Path, err)
		}
		if outputPath == "." || outputPath == "" || path.IsAbs(outputPath) || outputPath == ".." || strings.HasPrefix(outputPath, "../") {
			return nil, fmt.Errorf("unsafe output path %q for %q", outputPath, entry.Path)
		}
		if r.config.matcher(entry) {
			content, err = r.engine.Render(ctx, libtemplate.Input{Name: entry.Path, Source: content}, data)
			if err != nil {
				if continueOnError {
					results = append(results, Result{Source: entry.Source, InputPath: entry.Path, Path: outputPath, Error: err})
					renderErrors = append(renderErrors, fmt.Errorf("render %q from %q: %w", entry.Path, entry.Source, err))
					continue
				}
				return nil, err
			}
		}
		results = append(results, Result{Source: entry.Source, InputPath: entry.Path, Path: outputPath, Content: content})
	}
	return results, errors.Join(renderErrors...)
}
