/*
Purpose: Interpret supported rollout strategy evidence for restart.
Used by: Native restart inspection.
Responsibilities: Recognize documented defaults and preserve optional reported counts.
Important: Restart eligibility and observed counts do not imply readiness or convergence.
*/
package operation

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/sandjaie/KubeSift/internal/model"
)

func jsonCount(value any) *int32 {
	number, ok := value.(json.Number)
	if !ok {
		return nil
	}
	n, err := strconv.ParseInt(string(number), 10, 32)
	if err != nil || n < 0 {
		return nil
	}
	count := int32(n)
	return &count
}

func restartLimit(rolling map[string]any, key, fallback string) (string, bool, error) {
	raw, exists := rolling[key]
	if !exists {
		return fallback, fallback != "", nil
	}
	if n := jsonCount(raw); n != nil {
		return strconv.FormatInt(int64(*n), 10), false, nil
	}
	text, ok := raw.(string)
	if ok && strings.HasSuffix(text, "%") {
		if _, err := parseReplicas(strings.TrimSuffix(text, "%")); err == nil {
			return text, false, nil
		}
	}
	return "", false, invalidRestart()
}

func restartImpact(kind string, spec, status map[string]any) (RestartImpact, error) {
	impact := RestartImpact{Strategy: "RollingUpdate", StrategyDefault: true}
	unsupported := func(message string) (RestartImpact, error) {
		return RestartImpact{}, model.Fail("unsupported_strategy", message)
	}
	field := "updateStrategy"
	if kind == "Deployment" {
		field = "strategy"
		if value, exists := spec["paused"]; exists {
			paused, ok := value.(bool)
			if !ok {
				return RestartImpact{}, invalidRestart()
			}
			if paused {
				return unsupported("Resume the paused Deployment before preparing a restart.")
			}
		}
	}
	var strategy, rolling map[string]any
	if raw, has := spec[field]; has {
		var ok bool
		if strategy, ok = raw.(map[string]any); !ok {
			return RestartImpact{}, invalidRestart()
		}
		if rawType, exists := strategy["type"]; exists {
			var ok bool
			impact.Strategy, ok = rawType.(string)
			if !ok || impact.Strategy == "" {
				return RestartImpact{}, invalidRestart()
			}
			impact.StrategyDefault = false
		}
		if raw, exists := strategy["rollingUpdate"]; exists && raw != nil {
			if rolling, ok = raw.(map[string]any); !ok {
				return RestartImpact{}, invalidRestart()
			}
		}
	}
	impact.Recreate = kind == "Deployment" && impact.Strategy == "Recreate"
	if !impact.Recreate && impact.Strategy != "RollingUpdate" {
		return unsupported("Restart is unavailable for this controller strategy. RollingUpdate is required for StatefulSets and DaemonSets.")
	}
	if kind == "DaemonSet" {
		impact.Desired = jsonCount(status["desiredNumberScheduled"])
		impact.Current = jsonCount(status["currentNumberScheduled"])
		impact.Ready = jsonCount(status["numberReady"])
	} else {
		count, exists := spec["replicas"]
		if !exists {
			one := int32(1)
			impact.Desired, impact.DesiredDefault = &one, true
		} else if impact.Desired = jsonCount(count); impact.Desired == nil {
			return RestartImpact{}, invalidRestart()
		}
		impact.Current = jsonCount(status["replicas"])
		impact.Ready = jsonCount(status["readyReplicas"])
	}
	impact.ZeroPods = impact.Desired != nil && *impact.Desired == 0 && (impact.Current == nil || *impact.Current == 0)
	if impact.Recreate {
		return impact, nil
	}
	surge, unavailable := "", ""
	switch kind {
	case "Deployment":
		surge, unavailable = "25%", "25%"
	case "DaemonSet":
		surge, unavailable = "0", "1"
	case "StatefulSet":
		if raw, exists := rolling["partition"]; exists {
			impact.Partition = jsonCount(raw)
			if impact.Partition == nil {
				return RestartImpact{}, invalidRestart()
			}
			if *impact.Partition != 0 {
				return unsupported("Restart is unavailable while the StatefulSet rolling update partition is nonzero.")
			}
		} else {
			zero := int32(0)
			impact.Partition, impact.PartitionDefault = &zero, true
		}
	}
	var err error
	if kind != "StatefulSet" {
		impact.MaxSurge, impact.MaxSurgeDefault, err = restartLimit(rolling, "maxSurge", surge)
		if err != nil {
			return RestartImpact{}, err
		}
	}
	impact.MaxUnavailable, impact.MaxUnavailableDefault, err = restartLimit(rolling, "maxUnavailable", unavailable)
	return impact, err
}
