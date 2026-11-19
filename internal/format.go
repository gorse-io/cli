// Copyright 2026 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorse-io/gorse/client"
	"github.com/spf13/cobra"
)

const (
	maxInlineArrayValues     = 8
	maxTableLabelArrayValues = 3
)

func sortFeedback(feedback []client.Feedback) {
	sort.Slice(feedback, func(i, j int) bool {
		return feedback[i].Timestamp.After(feedback[j].Timestamp)
	})
}

func getConfigValue(configMap map[string]any, names ...string) (string, any, bool) {
	for _, name := range names {
		value, ok := configMap[name]
		if ok {
			return name, value, true
		}
	}
	return "", nil, false
}

func formatConfigMap(configMap map[string]any) map[string]any {
	formatted := make(map[string]any, len(configMap))
	for key, value := range configMap {
		formatted[key] = formatConfigValue(value)
	}
	return formatted
}

func formatConfigValue(value any) any {
	switch v := value.(type) {
	case time.Duration:
		s := v.String()
		if strings.HasSuffix(s, "m0s") {
			s = s[:len(s)-2]
		}
		if strings.HasSuffix(s, "h0m") {
			s = s[:len(s)-2]
		}
		return s
	case map[string]any:
		return formatConfigMap(v)
	case []any:
		formatted := make([]any, len(v))
		for i, item := range v {
			formatted[i] = formatConfigValue(item)
		}
		return formatted
	default:
		return v
	}
}

func nextPage(cmd *cobra.Command, cursor string, n int) {
	if cursor == "" {
		return
	}
	parts := []string{cmd.CommandPath(), "-n", fmt.Sprint(n), "--cursor", cursor}
	for _, name := range []string{"application", "endpoint"} {
		if cmd.Flags().Changed(name) {
			value, _ := cmd.Flags().GetString(name)
			parts = append(parts, "--"+name, value)
		}
	}
	if cmd.Flags().Changed("api-key") {
		parts = append(parts, "--api-key", "YOUR_API_KEY")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nNext page: %s\n", strings.Join(parts, " "))
}

func summarizeLongArrays(value any) any {
	return summarizeLongArraysWithLimit(value, maxInlineArrayValues)
}

func summarizeLongArraysWithLimit(value any, maxValues int) any {
	if summary, ok := formatArraySummaryWithLimit(value, maxValues); ok {
		return summary
	}
	if array, ok := sliceValues(value); ok {
		values := make([]any, len(array))
		for i, element := range array {
			values[i] = summarizeLongArraysWithLimit(element, maxValues)
		}
		return values
	}
	if object, ok := objectFields(value); ok {
		values := make(map[string]any, len(object))
		for key, element := range object {
			values[key] = summarizeLongArraysWithLimit(element, maxValues)
		}
		return values
	}
	return value
}

func formatArraySummary(value any) (string, bool) {
	return formatArraySummaryWithLimit(value, maxInlineArrayValues)
}

func formatArraySummaryWithLimit(value any, maxValues int) (string, bool) {
	array, ok := sliceValues(value)
	if maxValues < 1 {
		maxValues = 1
	}
	if !ok || len(array) <= maxValues || !isScalarArray(array) {
		return "", false
	}
	values := make([]string, 0, maxValues)
	for _, element := range array[:maxValues] {
		values = append(values, formatSummaryValue(element))
	}
	return fmt.Sprintf("[%s, ...] (%d values)", strings.Join(values, ", "), len(array)), true
}

func isScalarArray(array []any) bool {
	for _, element := range array {
		switch indirectInterface(element).(type) {
		case nil, string, json.Number, bool, float64, float32, int, int64, int32, uint, uint64, uint32, time.Time:
		default:
			return false
		}
	}
	return true
}

func formatSummaryValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return formatTableValue(value)
	}
	return string(encoded)
}

func formatProgressBar(count, total int) string {
	const width = 20
	if count < 0 {
		count = 0
	}
	if total < 0 {
		total = 0
	}
	if total > 0 && count > total {
		count = total
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(count) / float64(total)
	}
	filled := int(ratio*width + 0.5)
	if filled > width {
		filled = width
	}
	percent := int(ratio*100 + 0.5)
	return fmt.Sprintf("[%s%s] %d%%",
		strings.Repeat("#", filled),
		strings.Repeat("-", width-filled),
		percent)
}

func formatTableLabels(value any) string {
	if _, ok := sliceValues(value); ok || isObjectValue(value) {
		encoded, err := json.Marshal(summarizeLongArraysWithLimit(value, maxTableLabelArrayValues))
		if err != nil {
			return formatTableValue(value)
		}
		return string(encoded)
	}
	return formatTableValue(value)
}

func formatTableValue(value any) string {
	value = indirectInterface(value)
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		if formatted, ok := formatTimeValue(typed); ok {
			return formatted
		}
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case time.Time:
		return typed.Format(time.RFC3339)
	}
	if _, ok := sliceValues(value); ok {
		if summary, ok := formatArraySummary(value); ok {
			return summary
		}
		encoded, err := json.Marshal(summarizeLongArrays(value))
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
	if isObjectValue(value) {
		encoded, err := json.Marshal(summarizeLongArrays(value))
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
	return fmt.Sprint(value)
}

func formatTimeValue(value string) (string, bool) {
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", false
	}
	return timestamp.Format(time.RFC3339), true
}

func isObjectValue(value any) bool {
	_, ok := objectFields(value)
	return ok
}

func sliceValues(value any) ([]any, bool) {
	v := indirectValue(reflect.ValueOf(value))
	if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
		return nil, false
	}
	values := make([]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		values[i] = valueFromReflect(v.Index(i))
	}
	return values, true
}

func objectFields(value any) (map[string]any, bool) {
	return objectFieldsFromValue(reflect.ValueOf(value))
}

func objectFieldsFromValue(v reflect.Value) (map[string]any, bool) {
	v = indirectValue(v)
	if !v.IsValid() {
		return nil, false
	}
	if v.CanInterface() {
		if _, ok := v.Interface().(time.Time); ok {
			return nil, false
		}
	}
	if v.Type() == reflect.TypeOf(time.Time{}) {
		return nil, false
	}
	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		object := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			object[iter.Key().String()] = valueFromReflect(iter.Value())
		}
		return object, true
	case reflect.Struct:
		object := make(map[string]any, v.NumField())
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" && !field.Anonymous {
				continue
			}
			fieldValue := v.Field(i)
			if field.Anonymous {
				if embedded, ok := objectFieldsFromValue(fieldValue); ok {
					for key, value := range embedded {
						object[key] = value
					}
					continue
				}
			}
			object[field.Name] = valueFromReflect(fieldValue)
		}
		return object, true
	default:
		return nil, false
	}
}

func valueFromReflect(v reflect.Value) any {
	v = indirectValue(v)
	if !v.IsValid() {
		return nil
	}
	if v.CanInterface() {
		return v.Interface()
	}
	return fmt.Sprint(v)
}

func indirectInterface(value any) any {
	return valueFromReflect(reflect.ValueOf(value))
}

func indirectValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}
