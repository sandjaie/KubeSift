/*
Purpose: Convert editable YAML into a minimal conditional JSON patch.
Used by: The native edit preview and execution service.
Responsibilities: Preserve exact numbers, validate identity, and normalize Secret stringData.
Important: Server-owned fields omitted from the editor never become deletions.
*/
package operation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"go.yaml.in/yaml/v3"
)

type editPatch struct {
	body     []byte
	before   map[string]any
	proposed map[string]any
}

func invalidEditYAML() error {
	return model.Fail("invalid_yaml", "Enter one YAML object with unique string keys and supported scalar values. Aliases and custom tags are unsupported.")
}

func parseEditYAML(input string) (map[string]any, error) {
	if len(input) > editLimit {
		return nil, model.Fail("capacity", "YAML exceeds the 2 MiB size limit")
	}
	var document, extra yaml.Node
	decoder := yaml.NewDecoder(strings.NewReader(input))
	if err := decoder.Decode(&document); err != nil || len(document.Content) != 1 || !errors.Is(decoder.Decode(&extra), io.EOF) {
		return nil, invalidEditYAML()
	}
	value, err := editYAMLValue(document.Content[0], 0)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalidEditYAML()
	}
	return object, nil
}

var decimalInteger = regexp.MustCompile(`^[+-]?[0-9]+$`)
var pointerEscape = strings.NewReplacer("~", "~0", "/", "~1")

func editYAMLValue(node *yaml.Node, depth int) (any, error) {
	if depth > 64 || node.Anchor != "" {
		return nil, invalidEditYAML()
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" {
			return nil, invalidEditYAML()
		}
		value := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Anchor != "" {
				return nil, invalidEditYAML()
			}
			if _, duplicate := value[key.Value]; duplicate {
				return nil, invalidEditYAML()
			}
			child, err := editYAMLValue(node.Content[i+1], depth+1)
			if err != nil {
				return nil, err
			}
			value[key.Value] = child
		}
		return value, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return nil, invalidEditYAML()
		}
		value := make([]any, 0, len(node.Content))
		for _, item := range node.Content {
			child, err := editYAMLValue(item, depth+1)
			if err != nil {
				return nil, err
			}
			value = append(value, child)
		}
		return value, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!timestamp":
			return node.Value, nil
		case "!!null":
			if slices.Contains([]string{"", "~", "null", "Null", "NULL"}, node.Value) {
				return nil, nil
			}
		case "!!bool":
			switch strings.ToLower(node.Value) {
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
		case "!!int", "!!float":
			return exactEditNumber(node)
		}
	}
	return nil, invalidEditYAML()
}

func exactEditNumber(node *yaml.Node) (json.Number, error) {
	value := strings.ReplaceAll(node.Value, "_", "")
	if node.Tag == "!!int" || decimalInteger.MatchString(value) {
		base := 10
		// Keep quoted/explicit or implicit YAML octal and hexadecimal syntax consistent with the node resolver.
		if node.Tag == "!!int" {
			base = 0
		}
		if integer, ok := new(big.Int).SetString(value, base); ok {
			return json.Number(integer.String()), nil
		}
		return "", invalidEditYAML()
	}
	value = strings.TrimPrefix(value, "+")
	if strings.HasPrefix(value, ".") {
		value = "0" + value
	}
	if strings.HasPrefix(value, "-.") {
		value = "-0" + value[1:]
	}
	if mantissa, exponent, found := strings.Cut(strings.ToLower(value), "e"); found {
		if strings.HasSuffix(mantissa, ".") {
			value = mantissa + "0e" + exponent
		}
	} else if strings.HasSuffix(value, ".") {
		value += "0"
	}
	var number json.Number
	if json.Unmarshal([]byte(value), &number) != nil || number == "" || value[0] == '"' {
		return "", invalidEditYAML()
	}
	return number, nil
}

func normalizeSecretEdit(target kube.EditTarget, object map[string]any) error {
	if target.Group != "" || target.Version != "v1" || target.Resource != "secrets" {
		return nil
	}
	raw, present := object["stringData"]
	if !present {
		return nil
	}
	delete(object, "stringData")
	if raw == nil {
		return nil
	}
	text, ok := raw.(map[string]any)
	if !ok {
		return model.Fail("invalid_yaml", "Secret stringData must map keys to text")
	}
	if len(text) == 0 {
		return nil
	}
	data, ok := object["data"].(map[string]any)
	if !ok {
		if object["data"] != nil {
			return model.Fail("invalid_yaml", "Secret data must be a mapping")
		}
		data = map[string]any{}
		object["data"] = data
	}
	for key, value := range text {
		str, ok := value.(string)
		if !ok {
			return model.Fail("invalid_yaml", "Secret stringData values must be text")
		}
		data[key] = base64.StdEncoding.EncodeToString([]byte(str))
	}
	return nil
}

func buildEditPatch(capture kube.EditCapture, editedYAML string) (editPatch, error) {
	before, err := editProjection(capture.ObjectJSON)
	if err != nil {
		return editPatch{}, err
	}
	after, err := parseEditYAML(editedYAML)
	if err != nil {
		return editPatch{}, err
	}
	metadata, ok := after["metadata"].(map[string]any)
	original := before["metadata"].(map[string]any)
	if !ok || !reflect.DeepEqual(before["kind"], after["kind"]) || !reflect.DeepEqual(before["apiVersion"], after["apiVersion"]) {
		return editPatch{}, model.Fail("identity_changed", "Keep the resource API, kind and metadata identity unchanged")
	}
	for _, key := range []string{"name", "namespace", "uid", "resourceVersion"} {
		if !reflect.DeepEqual(original[key], metadata[key]) {
			return editPatch{}, model.Fail("identity_changed", "Keep the resource name, namespace, UID and version unchanged")
		}
	}
	if _, exists := after["status"]; exists {
		return editPatch{}, model.Fail("invalid_yaml", "Status is server-owned. Remove it from the edit")
	}
	for _, key := range serverEditMetadata {
		if _, exists := metadata[key]; exists {
			return editPatch{}, model.Fail("invalid_yaml", "Remove server-owned metadata from the edit")
		}
	}
	if err := normalizeSecretEdit(capture.Target, after); err != nil {
		return editPatch{}, err
	}
	if raw, err := json.Marshal(after); err != nil || len(raw) > editLimit {
		return editPatch{}, model.Fail("capacity", "Proposed content exceeds the 2 MiB limit")
	}
	var body bytes.Buffer
	body.WriteByte('[')
	appendOp := func(op, path string, value any) error {
		var raw json.RawMessage
		var err error
		if op != "remove" {
			raw, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}
		entry, err := json.Marshal(struct {
			Op    string          `json:"op"`
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value,omitempty"`
		}{op, path, raw})
		if err != nil || body.Len()+len(entry)+2 > editLimit {
			return model.Fail("capacity", "JSON Patch exceeds the 2 MiB limit")
		}
		if body.Len() > 1 {
			body.WriteByte(',')
		}
		body.Write(entry)
		return nil
	}
	if err := appendOp("test", "/metadata/uid", capture.Target.UID); err != nil {
		return editPatch{}, err
	}
	if err := appendOp("test", "/metadata/resourceVersion", capture.ResourceVersion); err != nil {
		return editPatch{}, err
	}
	beforeDiff := body.Len()
	if err := diffEditMaps("", before, after, appendOp); err != nil {
		return editPatch{}, err
	}
	if body.Len() == beforeDiff {
		return editPatch{}, model.Fail("no_changes", "There are no changes to review")
	}
	body.WriteByte(']')
	return editPatch{body: body.Bytes(), before: before, proposed: after}, nil
}

func diffEditMaps(path string, before, after map[string]any, appendOp func(string, string, any) error) error {
	for _, key := range slices.Sorted(maps.Keys(before)) {
		if _, exists := after[key]; !exists {
			if err := appendOp("remove", path+"/"+pointerEscape.Replace(key), nil); err != nil {
				return err
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(after)) {
		value, current := after[key], path+"/"+pointerEscape.Replace(key)
		previous, exists := before[key]
		oldMap, oldObject := previous.(map[string]any)
		newMap, newObject := value.(map[string]any)
		switch {
		case !exists:
			if err := appendOp("add", current, value); err != nil {
				return err
			}
		case oldObject && newObject:
			if err := diffEditMaps(current, oldMap, newMap, appendOp); err != nil {
				return err
			}
		case !reflect.DeepEqual(previous, value):
			if err := appendOp("replace", current, value); err != nil {
				return err
			}
		}
	}
	return nil
}
