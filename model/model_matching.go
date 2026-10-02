package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ModelMatchRule contains literal text only. Each include must match, while any
// exclude rejects the name. Expressions are generated and escaped by the server.
type ModelMatchRule struct {
	Include       []string `json:"include"`
	Exclude       []string `json:"exclude"`
	CaseSensitive bool     `json:"case_sensitive"`
}

func (rule *ModelMatchRule) Value() (driver.Value, error) {
	if rule == nil {
		return nil, nil
	}
	value, err := common.Marshal(rule)
	return string(value), err
}

func (rule *ModelMatchRule) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	var data []byte
	switch value := value.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return errors.New("invalid model match rule storage")
	}
	return common.Unmarshal(data, rule)
}

func (rule *ModelMatchRule) Normalize() error {
	if rule == nil {
		return nil
	}
	for _, terms := range []*[]string{&rule.Include, &rule.Exclude} {
		if len(*terms) > 32 {
			return errors.New("a model match rule supports up to 32 terms")
		}
		normalized := make([]string, 0, len(*terms))
		seen := make(map[string]bool)
		for _, term := range *terms {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}
			if len(term) > 128 {
				return errors.New("model match terms must be at most 128 bytes")
			}
			key := term
			if !rule.CaseSensitive {
				key = strings.ToLower(key)
			}
			if !seen[key] {
				normalized = append(normalized, term)
				seen[key] = true
			}
		}
		*terms = normalized
	}
	if len(rule.Include) == 0 {
		return errors.New("at least one included text is required")
	}
	return nil
}

type ModelMatcher struct {
	include []*regexp.Regexp
	exclude []*regexp.Regexp
}

func (mi *Model) CompileMatcher() (*ModelMatcher, error) {
	matcher := &ModelMatcher{}
	if mi.MatchRule == nil {
		// Existing imported records retain their scope until edited with literal
		// filters. New records use MatchRule and never accept a raw expression.
		pattern := regexp.QuoteMeta(mi.ModelName)
		switch mi.NameRule {
		case NameRuleExact:
			pattern = "^" + pattern + "$"
		case NameRulePrefix:
			pattern = "^" + pattern
		case NameRuleSuffix:
			pattern += "$"
		case NameRuleContains:
		default:
			return nil, errors.New("invalid model name rule")
		}
		matcher.include = []*regexp.Regexp{regexp.MustCompile(pattern)}
		return matcher, nil
	}
	if err := mi.MatchRule.Normalize(); err != nil {
		return nil, err
	}
	for i, terms := range [][]string{mi.MatchRule.Include, mi.MatchRule.Exclude} {
		for _, term := range terms {
			pattern := regexp.QuoteMeta(term)
			if !mi.MatchRule.CaseSensitive {
				pattern = "(?i)" + pattern
			}
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("compile model match text: %w", err)
			}
			if i == 0 {
				matcher.include = append(matcher.include, compiled)
			} else {
				matcher.exclude = append(matcher.exclude, compiled)
			}
		}
	}
	return matcher, nil
}

func (matcher *ModelMatcher) Matches(name string) bool {
	for _, pattern := range matcher.include {
		if !pattern.MatchString(name) {
			return false
		}
	}
	for _, pattern := range matcher.exclude {
		if pattern.MatchString(name) {
			return false
		}
	}
	return true
}
