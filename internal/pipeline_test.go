package internal

import (
	"encoding/json"

	"github.com/go-viper/mapstructure/v2"
)

func (s *CLITestSuite) TestGet() {
	var fields map[string]any
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &fields, Deep: true})
	s.Require().NoError(err)
	s.Require().NoError(decoder.Decode(s.master.Config.Recommend))
	expected, err := json.Marshal(formatConfigMap(fields))
	s.Require().NoError(err)
	s.Require().YAMLEq(string(expected), s.Command("pipeline", "get"))
}
