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
	"encoding"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

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

func formatIndentJSON(value any) string {
	return formatJSON(value, jsontext.WithIndent("  "))
}

func formatInlineJSON(value any) string {
	return formatJSON(value)
}

func formatJSON(value any, options ...json.Options) string {
	const placeholder = "true"
	type preview struct {
		offset int
		values []byte
	}
	var previews []preview
	marshalPreview := json.MarshalToFunc(func(enc *jsontext.Encoder, value any) error {
		switch value.(type) {
		case json.Marshaler, json.MarshalerTo, encoding.TextAppender, encoding.TextMarshaler:
			return errors.ErrUnsupported
		}
		array := reflect.ValueOf(value).Elem()
		if !isLongFloatArray(array) {
			return errors.ErrUnsupported
		}
		values, err := json.Marshal([]any{array.Index(0).Interface(), array.Index(1).Interface()})
		if err != nil {
			return err
		}
		// A non-empty placeholder keeps fields tagged omitempty.
		if err := enc.WriteToken(jsontext.True); err != nil {
			return err
		}
		previews = append(previews, preview{int(enc.OutputOffset()) - len(placeholder), values})
		return nil
	})
	options = append(options, json.Deterministic(true), json.WithMarshalers(marshalPreview))
	encoded := lo.Must(json.Marshal(value, options...))
	if len(previews) == 0 {
		return string(encoded)
	}
	// Replace only the placeholders emitted by the preview marshaler.
	var output strings.Builder
	start := 0
	for _, preview := range previews {
		output.Write(encoded[start:preview.offset])
		output.Write(preview.values[:len(preview.values)-1])
		output.WriteString(",...]")
		start = preview.offset + len(placeholder)
	}
	output.Write(encoded[start:])
	return output.String()
}

func isLongFloatArray(array reflect.Value) bool {
	if (array.Kind() != reflect.Array && array.Kind() != reflect.Slice) || array.Len() <= 2 {
		return false
	}
	switch array.Type().Elem().Kind() {
	case reflect.Float32, reflect.Float64:
		return true
	case reflect.Interface:
		// JSON numbers decoded into an interface are float64 values.
		for i := 0; i < array.Len(); i++ {
			kind := array.Index(i).Elem().Kind()
			if kind != reflect.Float32 && kind != reflect.Float64 {
				return false
			}
		}
		return true
	}
	return false
}
