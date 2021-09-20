/*
Copyright 2023 The RedisOperator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package user

import (
	"reflect"
	"testing"
)

// Add the Rule struct and Parse function here...

func TestParse(t *testing.T) {
	// Create Rule struct instance
	rule := Rule{}

	// Test case
	testCase := "allkeys ~test &* +@all +get -set"

	// Call Parse function to parse the rule
	err := rule.Parse(testCase)

	// Check for errors
	if err != nil {
		t.Errorf("Error parsing rule: %v", err)
	}

	// Check the expected values after parsing
	expectedRule := Rule{
		Categories:           []string{"all"},
		AllowedCommands:      []string{"get"},
		DisallowedCommands:   []string{"set"},
		DisallowedCategories: []string{""},
		KeyPatterns:          []string{"allkeys", "test"},
		Channels:             []string{"*"},
	}

	// Compare the parsed rule with the expected rule
	if reflect.DeepEqual(rule, expectedRule) {
		t.Errorf("Parsed rule does not match the expected rule. Parsed: %+v, Expected: %+v", rule, expectedRule)
	}
}
