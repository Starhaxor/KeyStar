// Package metrics implements a dependency-free Prometheus text-format
// registry covering the observability needs of the backend: request
// counters, latency histograms and gauges such as the webhook outbox
// backlog. Intentionally small — swap for a full client if requirements
// grow beyond counters/gauges/histograms.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Labels identifies one series of a metric.
type Labels map[string]string

func seriesKey(name string, labels Labels) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for label := range labels {
		keys = append(keys, label)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, label := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", label, escape(labels[label])))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

func escape(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return replacer.Replace(value)
}

type series struct {
	help    string
	mtype   string
	value   float64
	buckets []float64
	counts  map[string]uint64 // histogram buckets: le -> count (cumulative filled at render)
	sum     float64
	total   uint64
}

// Registry stores metric series and renders them in Prometheus format.
type Registry struct {
	mu      sync.Mutex
	series  map[string]*series
	histBks []float64
}

// NewRegistry builds a registry with default latency buckets.
func NewRegistry() *Registry {
	return &Registry{
		series: make(map[string]*series),
		histBks: []float64{
			0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		},
	}
}

// DeclareCounter registers a counter with documentation text.
func (r *Registry) DeclareCounter(name, help string) {
	r.declare(name, help, "counter")
}

// DeclareGauge registers a gauge with documentation text.
func (r *Registry) DeclareGauge(name, help string) {
	r.declare(name, help, "gauge")
}

// DeclareHistogram registers a cumulative histogram with documentation text.
func (r *Registry) DeclareHistogram(name, help string) {
	r.declare(name, help, "histogram")
}

func (r *Registry) declare(name, help, mtype string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.series[name]
	if !ok {
		entry = &series{}
		r.series[name] = entry
	}
	entry.help = help
	entry.mtype = mtype
}

// IncCounter increments a counter series by one.
func (r *Registry) IncCounter(name string, labels Labels) {
	r.AddCounter(name, labels, 1)
}

// AddCounter increments a counter series by delta.
func (r *Registry) AddCounter(name string, labels Labels, delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := seriesKey(name, labels)
	entry, ok := r.series[key]
	if !ok {
		entry = &series{mtype: "counter"}
		r.series[key] = entry
	}
	entry.value += delta
}

// SetGauge overwrites a gauge series value.
func (r *Registry) SetGauge(name string, labels Labels, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := seriesKey(name, labels)
	entry, ok := r.series[key]
	if !ok {
		entry = &series{mtype: "gauge"}
		r.series[key] = entry
	}
	entry.value = value
}

// ObserveHistogram records one observation on a cumulative histogram.
func (r *Registry) ObserveHistogram(name string, labels Labels, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	base := r.series[name]
	if base == nil || base.mtype != "histogram" {
		base = &series{mtype: "histogram", buckets: r.histBks}
		r.series[name] = base
	}

	infinite := "+Inf"
	leLabels := func(le string) Labels {
		copied := make(Labels, len(labels)+1)
		for k, v := range labels {
			copied[k] = v
		}
		copied["le"] = le
		return copied
	}

	for _, bound := range r.histBks {
		if value <= bound {
			leKey := seriesKey(name+"_bucket", leLabels(formatFloat(bound)))
			r.observeBucket(leKey)
		}
	}
	infKey := seriesKey(name+"_bucket", leLabels(infinite))
	r.observeBucket(infKey)

	sumKey := seriesKey(name+"_sum", labels)
	sumEntry := r.series[sumKey]
	if sumEntry == nil {
		sumEntry = &series{mtype: "untyped"}
		r.series[sumKey] = sumEntry
	}
	sumEntry.value += value
	countKey := seriesKey(name+"_count", labels)
	countEntry := r.series[countKey]
	if countEntry == nil {
		countEntry = &series{mtype: "untyped"}
		r.series[countKey] = countEntry
	}
	countEntry.total++
}

func (r *Registry) observeBucket(key string) {
	entry, ok := r.series[key]
	if !ok {
		entry = &series{mtype: "untyped"}
		r.series[key] = entry
	}
	entry.total++
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

// WritePrometheus renders all series in the Prometheus text exposition
// format, grouped per metric family.
func (r *Registry) WritePrometheus(writer io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.series))
	for name := range r.series {
		names = append(names, name)
	}
	sort.Strings(names)

	writtenHelp := map[string]bool{}
	for _, key := range names {
		entry := r.series[key]
		family := familyName(key)
		if !writtenHelp[family] && entry.help != "" && entry.mtype != "untyped" {
			fmt.Fprintf(writer, "# HELP %s %s\n# TYPE %s %s\n", family, entry.help, family, entry.mtype)
			writtenHelp[family] = true
		}
		if entry.mtype == "histogram" || entry.total > 0 {
			fmt.Fprintf(writer, "%s %s\n", key, renderValue(entry))
			continue
		}
		fmt.Fprintf(writer, "%s %s\n", key, renderValue(entry))
	}
}

func familyName(seriesName string) string {
	if idx := strings.Index(seriesName, "{"); idx >= 0 {
		base := seriesName[:idx]
		seriesName = base
	}
	switch {
	case strings.HasSuffix(seriesName, "_bucket"), strings.HasSuffix(seriesName, "_sum"), strings.HasSuffix(seriesName, "_count"):
		return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(seriesName, "_bucket"), "_sum"), "_count")
	default:
		return seriesName
	}
}

func renderValue(entry *series) string {
	if entry.total > 0 && entry.mtype == "untyped" {
		return strconvFormatUint(entry.total)
	}
	return strconvFormatFloat(entry.value)
}

func strconvFormatFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

func strconvFormatUint(value uint64) string {
	return fmt.Sprintf("%d", value)
}

// Handler serves the registry as an HTTP endpoint.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.WritePrometheus(writer)
	})
}
