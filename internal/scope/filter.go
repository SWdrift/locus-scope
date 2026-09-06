package scope

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type FilterOperator string

const (
	FilterEqual    FilterOperator = "="
	FilterNotEqual FilterOperator = "!="
	FilterPrefix   FilterOperator = "^="
	FilterContains FilterOperator = "*="
	FilterRegexp   FilterOperator = "~="
)

type Predicate struct {
	Path     []string
	Metadata bool
	Operator FilterOperator
	Value    any
	regexp   *regexp.Regexp
}

func ParseFilters(arguments []string) ([]Predicate, error) {
	predicates := make([]Predicate, 0, len(arguments))
	for _, argument := range arguments {
		operator, index := findFilterOperator(argument)
		if index <= 0 {
			return nil, fmt.Errorf("invalid filter %q", argument)
		}
		field := argument[:index]
		valueText := argument[index+len(operator):]
		metadata := strings.HasPrefix(field, "@")
		if metadata {
			field = strings.TrimPrefix(field, "@")
		}
		parts := strings.Split(field, ".")
		for _, part := range parts {
			if part == "" {
				return nil, fmt.Errorf("invalid filter path %q", argument[:index])
			}
		}
		var value any
		if err := json.Unmarshal([]byte(valueText), &value); err != nil {
			value = valueText
		}
		predicate := Predicate{Path: parts, Metadata: metadata, Operator: FilterOperator(operator), Value: value}
		if predicate.Operator == FilterPrefix || predicate.Operator == FilterContains || predicate.Operator == FilterRegexp {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("filter %q requires a string value", argument)
			}
			if predicate.Operator == FilterRegexp {
				expression, err := regexp.Compile(text)
				if err != nil {
					return nil, fmt.Errorf("invalid regexp in filter %q: %w", argument, err)
				}
				predicate.regexp = expression
			}
		}
		predicates = append(predicates, predicate)
	}
	return predicates, nil
}

func Match(object, metadata map[string]any, predicates []Predicate) bool {
	for _, predicate := range predicates {
		root := object
		if predicate.Metadata {
			root = metadata
		}
		actual, found := nestedField(root, predicate.Path)
		if !found || !matchValue(actual, predicate) {
			return false
		}
	}
	return true
}

func findFilterOperator(argument string) (string, int) {
	for index := range len(argument) {
		for _, operator := range []string{"!=", "^=", "*=", "~=", "="} {
			if strings.HasPrefix(argument[index:], operator) {
				return operator, index
			}
		}
	}
	return "", -1
}

func nestedField(root map[string]any, path []string) (any, bool) {
	var current any = root
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func matchValue(actual any, predicate Predicate) bool {
	switch predicate.Operator {
	case FilterEqual:
		return jsonEqual(actual, predicate.Value)
	case FilterNotEqual:
		return !jsonEqual(actual, predicate.Value)
	case FilterPrefix, FilterContains, FilterRegexp:
		text, ok := actual.(string)
		if !ok {
			return false
		}
		expected := predicate.Value.(string)
		switch predicate.Operator {
		case FilterPrefix:
			return strings.HasPrefix(text, expected)
		case FilterContains:
			return strings.Contains(text, expected)
		default:
			return predicate.regexp.MatchString(text)
		}
	default:
		return false
	}
}

func jsonEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
