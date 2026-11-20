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
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatProgressBar(t *testing.T) {
	for _, tc := range []struct {
		name         string
		count, total int
		want         string
	}{
		{"not started", 0, 10, "[--------------------] 0%"},
		{"halfway", 5, 10, "[##########----------] 50%"},
		{"complete", 10, 10, "[####################] 100%"},
		{"count exceeds total", 11, 10, "[####################] 100%"},
		{"negative count", -1, 10, "[--------------------] 0%"},
		{"zero total", 5, 0, "[--------------------] 0%"},
		{"negative total", 5, -1, "[--------------------] 0%"},
		{"round half up", 1, 8, "[###-----------------] 13%"},
		{"round down", 1, 9, "[##------------------] 11%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, formatProgressBar(tc.count, tc.total))
		})
	}
}

type previewFloat float64

func (value previewFloat) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, []float64{float64(value), 2, 3})
}

func TestFormatJSONPreviewCustomElements(t *testing.T) {
	value := []previewFloat{1, 4, 7}
	for _, format := range []func(any) string{formatInlineJSON, formatIndentJSON} {
		require.Equal(t, `[[1,2,3],[4,2,3],...]`, format(value))
	}
}

func TestFormatJSONPreviewOmitempty(t *testing.T) {
	value := struct {
		Enabled bool      `json:"enabled"`
		Vector  []float64 `json:"vector,omitempty"`
	}{Enabled: true, Vector: []float64{1, 2, 3}}
	require.Equal(t, `{"enabled":true,"vector":[1,2,...]}`, formatInlineJSON(value))
	require.Equal(t, "{\n  \"enabled\": true,\n  \"vector\": [1,2,...]\n}", formatIndentJSON(value))
}

func TestFormatJSON(t *testing.T) {
	var decoded any
	require.NoError(t, json.Unmarshal([]byte(`[1,2,3]`), &decoded))
	value := map[string]any{
		"decoded": decoded,
		"floats":  [3]float32{0.1, 0.2, 0.3},
		"ints":    []int{1, 2, 3},
		"mixed":   []any{1.0, nil, "[1,2,...]"},
		"short":   []float64{1, 2},
	}
	require.Equal(t, `{"decoded":[1,2,...],"floats":[0.1,0.2,...],"ints":[1,2,3],"mixed":[1,null,"[1,2,...]"],"short":[1,2]}`, formatInlineJSON(value))
	require.Equal(t, `{
  "decoded": [1,2,...],
  "floats": [0.1,0.2,...],
  "ints": [
    1,
    2,
    3
  ],
  "mixed": [
    1,
    null,
    "[1,2,...]"
  ],
  "short": [
    1,
    2
  ]
}`, formatIndentJSON(value))
}
