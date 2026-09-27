// Package scan defines the scanning model: risk levels, clean targets,
// providers and the engine that drives them concurrently.
package scan

import (
	"encoding/json"
)

// RiskLevel classifies how dangerous it is to clean a target.
type RiskLevel string

const (
	// RiskSafe means pure cache: the tool rebuilds it automatically at
	// (almost) zero cost, no network required.
	RiskSafe RiskLevel = "safe"
	// RiskCaution means cache/index that must be re-downloaded or rebuilt,
	// costing time and bandwidth.
	RiskCaution RiskLevel = "caution"
	// RiskHigh means build outputs or dependency trees: removing them
	// requires re-install / re-build and may break offline environments.
	RiskHigh RiskLevel = "high"
	// RiskDangerous means user data or unrecoverable content (Docker
	// volumes, databases). Double confirmation is required.
	RiskDangerous RiskLevel = "dangerous"
)

// RiskLabel returns the Chinese label of a risk level.
func (r RiskLevel) RiskLabel() string {
	switch r {
	case RiskSafe:
		return "安全"
	case RiskCaution:
		return "谨慎"
	case RiskHigh:
		return "高风险"
	case RiskDangerous:
		return "危险"
	}
	return string(r)
}

// RiskOrder ranks risk levels for sorting and filtering.
func RiskOrder(r RiskLevel) int {
	switch r {
	case RiskSafe:
		return 0
	case RiskCaution:
		return 1
	case RiskHigh:
		return 2
	case RiskDangerous:
		return 3
	}
	return 4
}

// Clean methods supported by the cleaner.
const (
	MethodDir         = "dir"         // remove the whole directory
	MethodDirContents = "dirContents" // remove children, keep the directory
	MethodCommand     = "command"     // run an external command
	MethodGroup       = "group"       // clean all children
)

// Target is one cleanable unit found by a provider.
type Target struct {
	ID          string            `json:"id"`
	Tool        string            `json:"tool"`
	ToolTitle   string            `json:"toolTitle"`
	Category    string            `json:"category"`
	Title       string            `json:"title"`
	Path        string            `json:"path,omitempty"`
	Description string            `json:"description,omitempty"`
	Risk        RiskLevel         `json:"risk"`
	Size        int64             `json:"size"`
	Count       int64             `json:"count,omitempty"`
	Items       []*Target         `json:"items,omitempty"`
	Method      string            `json:"method,omitempty"`
	Cmd         []string          `json:"cmd,omitempty"`
	AllowedRoot string            `json:"allowedRoot,omitempty"` // protective prefix for dir methods
	Available   bool              `json:"available"`
	Note        string            `json:"note,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`
}

// WalkTargets visits t and all its children recursively.
func WalkTargets(t *Target, fn func(*Target)) {
	if t == nil {
		return
	}
	fn(t)
	for _, c := range t.Items {
		WalkTargets(c, fn)
	}
}

// SumSize returns the total size of a target: its own size or the sum of
// children when only children are cleanable.
func (t *Target) SumSize() int64 {
	if t.Size > 0 {
		return t.Size
	}
	var sum int64
	for _, c := range t.Items {
		sum += c.SumSize()
	}
	return sum
}

// Summary is the aggregated result of a full scan.
type Summary struct {
	TotalSize int64            `json:"totalSize"`
	ByRisk    map[string]int64 `json:"byRisk"`
	ByTool    map[string]int64 `json:"byTool"`
	Targets   int              `json:"targets"`
}

// Event is a progress notification emitted while scanning or cleaning.
// It is serialized as JSON over SSE and reused by the CLI printer.
type Event struct {
	Type    string       `json:"type"` // progress | target | done | log | result | error
	Tool    string       `json:"tool,omitempty"`
	Message string       `json:"message,omitempty"`
	Target  *Target      `json:"target,omitempty"`
	Summary *Summary     `json:"summary,omitempty"`
	Result  *CleanResult `json:"result,omitempty"`
}

// JSONString is a small helper for debugging.
func (e Event) JSONString() string {
	b, _ := json.Marshal(e)
	return string(b)
}
