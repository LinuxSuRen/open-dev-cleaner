package scan

import (
	"context"
	"sort"
	"sync"
)

// Provider scans one tool (or one family of tools) and emits clean targets
// as they are found. Providers must never block forever: honor ctx.
type Provider interface {
	Key() string
	Title() string
	Scan(ctx context.Context, env *Env, emit func(*Target))
}

// Engine runs all registered providers concurrently.
type Engine struct {
	Env       *Env
	Providers []Provider
}

// NewEngine builds an engine with the default provider set.
func NewEngine(env *Env) *Engine {
	return &Engine{Env: env, Providers: DefaultProviders()}
}

// Run executes every provider concurrently. emit receives every event in
// order: a "progress" event per provider start/finish, one "target" event
// per target (already sorted inside its provider), and a final "done" event
// carrying the summary. It returns the flat list of top-level targets.
func (e *Engine) Run(ctx context.Context, emit func(Event)) []*Target {
	var mu sync.Mutex
	targets := make([]*Target, 0, 32)
	var wg sync.WaitGroup

	for _, p := range e.Providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			mu.Lock()
			emit(Event{Type: "progress", Tool: p.Key(), Message: "scanning"})
			mu.Unlock()

			collected := make([]*Target, 0, 8)
			cb := func(t *Target) {
				if t == nil {
					return
				}
				collected = append(collected, t)
			}
			safeScan(ctx, e.Env, p, cb)

			sort.SliceStable(collected, func(i, j int) bool {
				return collected[i].SumSize() > collected[j].SumSize()
			})
			mu.Lock()
			for _, t := range collected {
				// Group targets carry the sum of their children so every
				// consumer sees a meaningful size directly.
				if len(t.Items) > 0 && t.Size == 0 {
					t.Size = t.SumSize()
				}
				targets = append(targets, t)
				emit(Event{Type: "target", Tool: p.Key(), Target: t})
			}
			emit(Event{Type: "progress", Tool: p.Key(), Message: "done"})
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	emit(Event{Type: "done", Summary: Summarize(targets)})
	return targets
}

// safeScan guards providers against panics so one bad provider cannot take
// down the whole engine.
func safeScan(ctx context.Context, env *Env, p Provider, cb func(*Target)) {
	defer func() {
		if r := recover(); r != nil {
			cb(&Target{
				ID: p.Key() + ":error", Tool: p.Key(), ToolTitle: p.Title(),
				Category: "错误", Title: p.Title() + " 扫描失败",
				Description: "扫描该工具时发生内部错误,已跳过",
				Available:   false,
			})
		}
	}()
	p.Scan(ctx, env, cb)
}

// Summarize aggregates a target list.
func Summarize(targets []*Target) *Summary {
	s := &Summary{ByRisk: map[string]int64{}, ByTool: map[string]int64{}}
	for _, t := range targets {
		size := t.SumSize()
		s.TotalSize += size
		s.ByRisk[string(t.Risk)] += size
		s.ByTool[t.Tool] += size
		s.Targets += countTargets(t)
	}
	return s
}

func countTargets(t *Target) int {
	n := 1
	for _, c := range t.Items {
		n += countTargets(c)
	}
	return n
}

// FindTarget resolves a target (or group) by ID from a scan result,
// searching children recursively.
func FindTarget(targets []*Target, id string) *Target {
	for _, t := range targets {
		if found := findIn(t, id); found != nil {
			return found
		}
	}
	return nil
}

func findIn(t *Target, id string) *Target {
	if t.ID == id {
		return t
	}
	for _, c := range t.Items {
		if found := findIn(c, id); found != nil {
			return found
		}
	}
	return nil
}
