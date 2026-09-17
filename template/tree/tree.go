// Package tree discovers, matches, and renders multi-source template file trees in memory,
// resolving file collisions and distinguishing dynamic templates from static file copies.
//
// A [Renderer] processes an ordered list of [Source] filesystems (such as embedded
// filesystems, local directory trees, or memory maps). It walks each source, evaluates
// collision resolution policies (e.g. prefer first, prefer last, or error), executes
// matched templates through a [libtemplate.Engine], and passes static files through unmodified.
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

// Source defines a named filesystem source and its root directories to discover files from.
type Source struct {
	// Name identifies the source layer (e.g. "base", "overlay").
	Name string
	// FS is the underlying filesystem containing the files.
	FS fs.FS
	// Roots defines the root directories within FS to walk. Defaults to ["."].
	Roots []string
}

// Entry represents a single file discovered from a Source.
type Entry struct {
	// Source is the name of the originating Source.
	Source string
	// SourceIndex is the 0-indexed position of the source in the configuration.
	SourceIndex int
	// Path is the cleaned relative file path within the source filesystem.
	Path string
	// FS is the source filesystem containing the file.
	FS fs.FS
}

// Matcher returns true if an Entry should be treated and rendered as a template.
type Matcher func(Entry) bool

// Predicate returns true if an Entry should be included in the rendering pipeline.
type Predicate func(Entry) bool

// PathFunc transforms an Entry's input path into its final output path.
type PathFunc func(Entry) (string, error)

// KeyFunc computes a unique key for an Entry used to detect and resolve collisions across sources.
type KeyFunc func(Entry) (string, error)

// CollisionResolver decides whether next should replace current when two entries have the same key.
type CollisionResolver func(current, next Entry) (replaceCurrent bool, err error)

// CollisionPolicy defines standard behavior when two sources produce the same output key.
type CollisionPolicy int

const (
	// CollisionError causes rendering to fail immediately if a collision is detected.
	CollisionError CollisionPolicy = iota
	// CollisionPreferFirst keeps the first entry discovered and discards subsequent entries.
	CollisionPreferFirst
	// CollisionPreferLast overwrites earlier entries with the latest discovered entry.
	CollisionPreferLast
)

// Result represents the outcome of rendering or copying a single file.
type Result struct {
	// Source is the name of the source that produced this file.
	Source string
	// InputPath is the original path of the file in the source filesystem.
	InputPath string
	// Path is the destination path for the rendered file.
	Path string
	// Content contains the rendered template or raw static file bytes.
	Content []byte
	// Error contains any failure that occurred while reading or rendering this file.
	Error error
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

// Option configures a Renderer instance.
type Option func(*config)

// WithSources registers one or more filesystem sources to walk and render.
func WithSources(sources ...Source) Option {
	return func(c *config) { c.sources = append(c.sources, sources...) }
}

// WithTemplateMatcher sets the predicate used to determine if a file is a template.
func WithTemplateMatcher(matcher Matcher) Option { return func(c *config) { c.matcher = matcher } }

// WithPredicate sets a filter to include or exclude files during discovery.
func WithPredicate(predicate Predicate) Option { return func(c *config) { c.predicate = predicate } }

// WithPathFunc sets the function that computes the output path for an entry.
func WithPathFunc(fn PathFunc) Option { return func(c *config) { c.path = fn } }

// WithKeyFunc sets the function that computes collision keys for entries.
func WithKeyFunc(fn KeyFunc) Option { return func(c *config) { c.key = fn } }

// WithCollisionPolicy sets the collision resolution policy when multiple entries share a key.
func WithCollisionPolicy(policy CollisionPolicy) Option {
	return func(c *config) { c.collision = policy }
}

// WithCollisionResolver sets a custom resolver function for entry collisions.
func WithCollisionResolver(resolver CollisionResolver) Option {
	return func(c *config) { c.resolver = resolver }
}

// Suffix returns a Matcher that matches files ending with the specified suffix (e.g. ".tplt").
func Suffix(suffix string) Matcher {
	return func(entry Entry) bool { return strings.HasSuffix(entry.Path, suffix) }
}

// Renderer walks configured filesystem sources, resolves collisions, renders templates,
// and copies static files in memory.
type Renderer struct {
	engine *libtemplate.Engine
	config config
}

// New creates a new Renderer with the given template engine and options.
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

// Render discovers files across configured sources, resolves collisions according to policy,
// renders matched template files against data, and copies static files directly.
//
// It returns an error immediately and stops if any template fails to parse or render,
// or if an unsafe output path is detected. For partial results with error accumulation,
// see [Renderer.RenderAll].
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
