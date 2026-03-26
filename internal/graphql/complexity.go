// Package graphql This file implements query validation utilities for GraphQL, analyzing query structure to calculate depth and complexity costs while enforcing execution limits.
package graphql

import (
	"encoding/json"
	"fmt"
	"strconv"

	gql "github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
)

const defaultComplexityListLimit = 25

// CheckDepth validates that a GraphQL query does not exceed the specified maximum nesting depth, returning an error if exceeded.
func CheckDepth(doc *ast.Document, maxDepth int) error {
	if doc == nil || maxDepth <= 0 {
		return nil
	}

	fragments := fragmentDefinitions(doc)
	maxSeen := 0
	for _, definition := range doc.Definitions {
		op, ok := definition.(*ast.OperationDefinition)
		if !ok || op.SelectionSet == nil {
			continue
		}
		walkDepthSelections(op.SelectionSet.Selections, 0, fragments, map[string]bool{}, &maxSeen)
	}

	if maxSeen > maxDepth {
		return fmt.Errorf("query depth %d exceeds maximum allowed depth of %d", maxSeen, maxDepth)
	}
	return nil
}

// walkDepthSelections recursively traverses a selection set to measure query depth, tracking the maximum nesting level encountered across all fields, inline fragments, and fragment spreads.
func walkDepthSelections(
	selections []ast.Selection,
	depth int,
	fragments map[string]*ast.FragmentDefinition,
	visiting map[string]bool,
	maxSeen *int,
) {
	for _, selection := range selections {
		switch node := selection.(type) {
		case *ast.Field:
			fieldDepth := depth + 1
			if fieldDepth > *maxSeen {
				*maxSeen = fieldDepth
			}
			if node.SelectionSet != nil {
				walkDepthSelections(node.SelectionSet.Selections, fieldDepth, fragments, visiting, maxSeen)
			}
		case *ast.InlineFragment:
			if node.SelectionSet != nil {
				walkDepthSelections(node.SelectionSet.Selections, depth, fragments, visiting, maxSeen)
			}
		case *ast.FragmentSpread:
			if node.Name == nil {
				continue
			}
			name := node.Name.Value
			frag := fragments[name]
			if frag == nil || visiting[name] || frag.SelectionSet == nil {
				continue
			}
			visiting[name] = true
			walkDepthSelections(frag.SelectionSet.Selections, depth, fragments, visiting, maxSeen)
			delete(visiting, name)
		}
	}
}

// checkComplexityWithVariables calculates the total complexity cost of a GraphQL document accounting for variable values, returning the total complexity and an error if it exceeds the maximum allowed.
func checkComplexityWithVariables(
	doc *ast.Document,
	schema *gql.Schema,
	maxComplexity int,
	variableValues map[string]interface{},
) (int, error) {
	if doc == nil || maxComplexity <= 0 {
		return 0, nil
	}
	if schema == nil {
		return 0, nil
	}

	fragments := fragmentDefinitions(doc)
	total := 0

	for _, definition := range doc.Definitions {
		op, ok := definition.(*ast.OperationDefinition)
		if !ok || op.SelectionSet == nil {
			continue
		}
		root := operationRootType(schema, op.Operation)
		if root == nil {
			continue
		}
		total += selectionSetComplexity(op.SelectionSet.Selections, root, schema, fragments, map[string]bool{}, variableValues)
	}

	if total > maxComplexity {
		return total, fmt.Errorf("query complexity %d exceeds maximum allowed complexity of %d", total, maxComplexity)
	}
	return total, nil
}

// selectionSetComplexity recursively sums the complexity cost of all selections, processing fields, inline fragments, and fragment spreads according to their nested costs.
func selectionSetComplexity(
	selections []ast.Selection,
	parentType *gql.Object,
	schema *gql.Schema,
	fragments map[string]*ast.FragmentDefinition,
	visiting map[string]bool,
	variableValues map[string]interface{},
) int {
	if parentType == nil {
		return 0
	}

	total := 0
	for _, selection := range selections {
		switch node := selection.(type) {
		case *ast.Field:
			total += fieldComplexity(node, parentType, schema, fragments, visiting, variableValues)
		case *ast.InlineFragment:
			if node.SelectionSet == nil {
				continue
			}
			inlineType := typeConditionObject(schema, node.TypeCondition, parentType)
			total += selectionSetComplexity(node.SelectionSet.Selections, inlineType, schema, fragments, visiting, variableValues)
		case *ast.FragmentSpread:
			if node.Name == nil {
				continue
			}
			name := node.Name.Value
			frag := fragments[name]
			if frag == nil || frag.SelectionSet == nil || visiting[name] {
				continue
			}
			fragmentType := typeConditionObject(schema, frag.TypeCondition, parentType)
			visiting[name] = true
			total += selectionSetComplexity(frag.SelectionSet.Selections, fragmentType, schema, fragments, visiting, variableValues)
			delete(visiting, name)
		}
	}
	return total
}

// fieldComplexity calculates the complexity cost of a single field, multiplying by the limit argument for list types and recursively summing child selection costs.
func fieldComplexity(
	field *ast.Field,
	parentType *gql.Object,
	schema *gql.Schema,
	fragments map[string]*ast.FragmentDefinition,
	visiting map[string]bool,
	variableValues map[string]interface{},
) int {
	if field == nil || field.Name == nil {
		return 0
	}

	name := field.Name.Value
	if isIntrospectionField(name) {
		return 1
	}

	fieldDef := parentType.Fields()[name]
	if fieldDef == nil {
		return 1
	}

	resolvedType := unwrapType(fieldDef.Type)
	childObject := nestedObjectType(resolvedType)
	childCost := 1
	if field.SelectionSet != nil && childObject != nil {
		childCost = selectionSetComplexity(field.SelectionSet.Selections, childObject, schema, fragments, visiting, variableValues)
		if childCost == 0 {
			childCost = 1
		}
	}

	if _, ok := resolvedType.(*gql.List); ok {
		limit := fieldLimitArg(field, variableValues)
		return limit * childCost
	}
	if childObject != nil {
		return childCost
	}
	return 1
}

// fieldLimitArg extracts the limit argument from a field, resolving variable references as needed, and returns the parsed limit or a default value if not specified.
func fieldLimitArg(field *ast.Field, variableValues map[string]interface{}) int {
	if field == nil {
		return defaultComplexityListLimit
	}
	for _, arg := range field.Arguments {
		if arg == nil || arg.Name == nil || arg.Value == nil || arg.Name.Value != "limit" {
			continue
		}
		if parsed, ok := parsePositiveIntValue(arg.Value); ok {
			return parsed
		}
		variable, ok := arg.Value.(*ast.Variable)
		if !ok || variable.Name == nil {
			continue
		}
		raw, exists := variableValues[variable.Name.Value]
		if !exists {
			continue
		}
		if parsed, ok := parsePositiveInt(raw); ok {
			return parsed
		}
	}
	return defaultComplexityListLimit
}

func parsePositiveIntValue(value ast.Value) (int, bool) {
	intVal, ok := value.(*ast.IntValue)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(intVal.Value)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

// parsePositiveInt converts a value of various types to a positive integer, supporting int, int64, float64, json.Number, and string formats, and returning the result with a success flag.
func parsePositiveInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v, true
		}
	case int64:
		if v > 0 && v <= int64(^uint(0)>>1) {
			return int(v), true
		}
	case float64:
		if v > 0 && v == float64(int(v)) {
			return int(v), true
		}
	case json.Number:
		i, err := strconv.Atoi(v.String())
		if err == nil && i > 0 {
			return i, true
		}
	case string:
		i, err := strconv.Atoi(v)
		if err == nil && i > 0 {
			return i, true
		}
	}
	return 0, false
}

func isIntrospectionField(name string) bool {
	switch name {
	case "__schema", "__type", "__typename":
		return true
	default:
		return false
	}
}

func operationRootType(schema *gql.Schema, op string) *gql.Object {
	switch op {
	case ast.OperationTypeMutation:
		return schema.MutationType()
	case ast.OperationTypeSubscription:
		return schema.SubscriptionType()
	case ast.OperationTypeQuery:
		fallthrough
	default:
		return schema.QueryType()
	}
}

func fragmentDefinitions(doc *ast.Document) map[string]*ast.FragmentDefinition {
	fragments := map[string]*ast.FragmentDefinition{}
	if doc == nil {
		return fragments
	}
	for _, definition := range doc.Definitions {
		frag, ok := definition.(*ast.FragmentDefinition)
		if !ok || frag.Name == nil {
			continue
		}
		fragments[frag.Name.Value] = frag
	}
	return fragments
}

func typeConditionObject(schema *gql.Schema, condition *ast.Named, fallback *gql.Object) *gql.Object {
	if schema == nil || condition == nil || condition.Name == nil {
		return fallback
	}
	resolved, ok := schema.Type(condition.Name.Value).(*gql.Object)
	if !ok || resolved == nil {
		return fallback
	}
	return resolved
}

func unwrapType(t gql.Type) gql.Type {
	for {
		nn, ok := t.(*gql.NonNull)
		if !ok {
			return t
		}
		t = nn.OfType
	}
}

func nestedObjectType(t gql.Type) *gql.Object {
	switch v := t.(type) {
	case *gql.Object:
		return v
	case *gql.List:
		return nestedObjectType(unwrapType(v.OfType))
	case *gql.NonNull:
		return nestedObjectType(v.OfType)
	default:
		return nil
	}
}
