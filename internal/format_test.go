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
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

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
