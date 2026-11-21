/*
Purpose: Project current usage with exact arithmetic and coverage.
Used by: Named Metrics API reads.
Responsibilities: Validate sample intervals, sum known containers and compare current configured resources.
Important: Pod totals have no percentage and incomplete coverage is labelled.
*/
package kube

import (
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func decimalText(value *big.Rat, places int) string {
	text := value.FloatString(places)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "-0" {
		return "0"
	}
	return text
}
func metricDecimal(text string, cpu bool) (*big.Rat, error) {
	q, err := resource.ParseQuantity(text)
	if err != nil || q.Sign() < 0 || len(text) > 128 {
		return nil, model.Fail("invalid_response", "Metrics contains an invalid quantity.")
	}
	decimal := q.AsDec()
	if decimal.Scale() < -128 || decimal.Scale() > 128 {
		return nil, model.Fail("invalid_response", "Metrics quantity exceeds the decimal range.")
	}
	value, ok := new(big.Rat).SetString(decimal.String())
	if !ok {
		return nil, model.Fail("invalid_response", "Metrics contains an invalid quantity.")
	}
	if cpu {
		value.Mul(value, big.NewRat(1000, 1))
	}
	return value, nil
}
func metricAmount(usage map[string]string, key string) (MetricValue, error) {
	var result MetricValue
	text, ok := usage[key]
	if !ok {
		return result, nil
	}
	value, err := metricDecimal(text, key == "cpu")
	if err != nil {
		return result, err
	}
	amount := decimalText(value, 9)
	result.Amount = &amount
	return result, nil
}
func compareMetric(value *MetricValue, quantity resource.Quantity, key string, cpu bool) {
	if quantity.Sign() <= 0 {
		return
	}
	number, err := metricDecimal(quantity.String(), cpu)
	if err != nil {
		return
	}
	text := decimalText(number, 9)
	var percent *string
	if value.Amount != nil {
		numerator, ok := new(big.Rat).SetString(*value.Amount)
		if ok {
			p := decimalText(new(big.Rat).Mul(new(big.Rat).Quo(numerator, number), big.NewRat(100, 1)), 6)
			percent = &p
		}
	}
	switch key {
	case "request":
		value.Request, value.RequestPercent = &text, percent
	case "limit":
		value.Limit, value.LimitPercent = &text, percent
	case "allocatable":
		value.Allocatable, value.AllocatablePercent = &text, percent
	}
}
func sumMetric(total *MetricValue, item MetricValue) {
	if item.Amount == nil {
		return
	}
	value, _ := new(big.Rat).SetString(*item.Amount)
	if total.Amount != nil {
		prior, _ := new(big.Rat).SetString(*total.Amount)
		value.Add(value, prior)
	}
	text := decimalText(value, 9)
	total.Amount = &text
}
func projectMetrics(target ResourceRef, sample metricsWire, pod *core.Pod, node *core.Node, detail bool, version string, now time.Time) (ResourceUsage, error) {
	out := ResourceUsage{Resource: target, APIVersion: version, FetchedAt: now, State: "available", CPUComplete: true, MemoryComplete: true, ConfiguredAt: &now}
	invalid := func() (ResourceUsage, error) {
		return out, model.Fail("invalid_response", "The Metrics API returned invalid or inconsistent sample evidence.")
	}
	stamp := sample.Timestamp.Time
	window, err := time.ParseDuration(sample.Window)
	if err != nil || window <= 0 || stamp.IsZero() || stamp.After(now.Add(30*time.Second)) {
		return invalid()
	}
	if sample.Metadata.Name != target.Name || sample.Metadata.Namespace != target.Namespace {
		return invalid()
	}
	if sample.Metadata.UID != "" && string(sample.Metadata.UID) != target.UID {
		return out, model.Fail("resource_replaced", "Metric identity does not match the selected resource.")
	}
	out.SampledAt = &stamp
	out.WindowSeconds = decimalText(new(big.Rat).SetFrac64(int64(window), int64(time.Second)), 9)
	created := time.Time{}
	if pod != nil {
		created = pod.CreationTimestamp.Time
		if string(pod.UID) != target.UID {
			return out, model.Fail("resource_replaced", "The Pod was replaced.")
		}
	}
	if node != nil {
		created = node.CreationTimestamp.Time
		if string(node.UID) != target.UID {
			return out, model.Fail("resource_replaced", "The Node was replaced.")
		}
	}
	if created.IsZero() || stamp.Add(-window).Before(created) {
		return out, model.Fail("stale_target", "The metrics interval predates the selected resource.")
	}
	if node != nil {
		out.CPU, err = metricAmount(sample.Usage, "cpu")
		if err != nil {
			return invalid()
		}
		out.Memory, err = metricAmount(sample.Usage, "memory")
		if err != nil {
			return invalid()
		}
		compareMetric(&out.CPU, node.Status.Allocatable[core.ResourceCPU], "allocatable", true)
		compareMetric(&out.Memory, node.Status.Allocatable[core.ResourceMemory], "allocatable", false)
		out.CPUComplete = out.CPU.Amount != nil
		out.MemoryComplete = out.Memory.Amount != nil
	} else if pod != nil {
		if len(sample.Containers) > 128 {
			return out, model.Fail("capacity", "The Pod metrics exceeds the container limit.")
		}
		type config struct {
			category  string
			resources core.ResourceRequirements
		}
		known := map[string]config{}
		for _, c := range pod.Spec.Containers {
			known[c.Name] = config{"application", c.Resources}
		}
		for _, c := range pod.Spec.InitContainers {
			known[c.Name] = config{"init", c.Resources}
		}
		for _, c := range pod.Spec.EphemeralContainers {
			known[c.Name] = config{"ephemeral", c.Resources}
		}
		running := map[string]bool{}
		for _, statuses := range [][]core.ContainerStatus{pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses, pod.Status.EphemeralContainerStatuses} {
			for _, c := range statuses {
				if c.State.Running != nil {
					running[c.Name] = true
				}
			}
		}
		seen := map[string]bool{}
		missing := map[string]bool{}
		for _, entry := range sample.Containers {
			cfg, ok := known[entry.Name]
			if !ok || entry.Name == "" || seen[entry.Name] {
				return invalid()
			}
			seen[entry.Name] = true
			cpu, e := metricAmount(entry.Usage, "cpu")
			if e != nil {
				return invalid()
			}
			mem, e := metricAmount(entry.Usage, "memory")
			if e != nil {
				return invalid()
			}
			if cpu.Amount == nil {
				out.CPUComplete = false
				missing[entry.Name] = true
			}
			if mem.Amount == nil {
				out.MemoryComplete = false
				missing[entry.Name] = true
			}
			sumMetric(&out.CPU, cpu)
			sumMetric(&out.Memory, mem)
			if detail {
				compareMetric(&cpu, cfg.resources.Requests[core.ResourceCPU], "request", true)
				compareMetric(&cpu, cfg.resources.Limits[core.ResourceCPU], "limit", true)
				compareMetric(&mem, cfg.resources.Requests[core.ResourceMemory], "request", false)
				compareMetric(&mem, cfg.resources.Limits[core.ResourceMemory], "limit", false)
				out.Containers = append(out.Containers, ContainerUsage{entry.Name, cfg.category, cpu, mem})
			}
		}
		for name := range running {
			if !seen[name] {
				out.CPUComplete = false
				out.MemoryComplete = false
				missing[name] = true
			}
		}
		for name := range missing {
			out.MissingContainers = append(out.MissingContainers, name)
		}
		sort.Strings(out.MissingContainers)
		out.PodLevelBudget = pod.Spec.Resources != nil
	} else {
		return invalid()
	}
	if out.CPU.Amount == nil {
		out.CPUComplete = false
	}
	if out.Memory.Amount == nil {
		out.MemoryComplete = false
	}
	if !out.CPUComplete || !out.MemoryComplete {
		out.State = "partial"
	}
	if out.CPU.Amount == nil && out.Memory.Amount == nil {
		out.State = "unavailable"
		out.Problem = &model.Problem{Code: "missing_sample", Message: "No usable CPU or memory sample was reported."}
	}
	if now.Sub(stamp) > 60*time.Second && out.State != "unavailable" {
		out.State = "stale"
	}
	if len(out.MissingContainers) > 128 {
		return out, model.Fail("capacity", "The Pod exceeds the running container limit.")
	}
	return out, nil
}
