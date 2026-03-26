// Package graphql Provides WebSocket subscription lifecycle management for GraphQL subscriptions, including request parsing, table validation, real-time event forwarding, and field projection filtering.
package graphql

import (
	"context"
	"fmt"
	"strconv"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
)

type gqlwsSubscriptionState struct {
	hubClientID string
	cancel      context.CancelFunc
	table       string
	fieldName   string
	where       map[string]any
	selection   map[string]*projectionField
}

type projectionField struct {
	source   string
	children map[string]*projectionField
}

// onWSSubscribe processes a GraphQL WebSocket subscription request by validating the hub and table, parsing the subscription, registering with the realtime hub, and starting event forwarding.
func (h *Handler) onWSSubscribe(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload) {
	if h.hub == nil {
		conn.removeSubscription(id)
		conn.SendError(id, []map[string]string{{"message": "realtime hub is not configured"}})
		return
	}
	sub, err := parseSubscriptionRequest(payload.Query, payload.OperationName, payload.Variables)
	if err != nil {
		conn.removeSubscription(id)
		conn.SendError(id, []map[string]string{{"message": err.Error()}})
		return
	}

	cache := h.getCache()
	if cache == nil {
		conn.removeSubscription(id)
		conn.SendError(id, []map[string]string{{"message": "unknown subscription table"}})
		return
	}
	tbl := cache.TableByName(sub.table)
	if tbl == nil || skipSubscriptionTable(tbl) {
		conn.removeSubscription(id)
		conn.SendError(id, []map[string]string{{"message": "unknown subscription table"}})
		return
	}

	hubClient := h.hub.Subscribe(map[string]bool{sub.table: true})
	subCtx, cancel := context.WithCancel(context.Background())
	state := &gqlwsSubscriptionState{
		hubClientID: hubClient.ID,
		cancel:      cancel,
		table:       sub.table,
		fieldName:   sub.fieldName,
		where:       sub.where,
		selection:   sub.selection,
	}
	h.storeSubscriptionState(conn.ID(), id, state)
	go h.forwardSubscriptionEvents(subCtx, conn, id, hubClient, state)
}

func (h *Handler) onWSComplete(conn *GQLWSConn, id string) {
	h.unsubscribeWS(conn.ID(), id)
}

func (h *Handler) onWSDisconnect(conn *GQLWSConn) {
	h.unsubscribeAllWS(conn.ID())
}

type parsedSubscriptionRequest struct {
	table     string
	fieldName string
	where     map[string]any
	selection map[string]*projectionField
}

// parseSubscriptionRequest parses a GraphQL subscription query and returns the table name, field name, where conditions, and field selection.
func parseSubscriptionRequest(query, operationName string, variables map[string]any) (*parsedSubscriptionRequest, error) {
	doc, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		return nil, fmt.Errorf("invalid subscription query")
	}
	op := selectedOperationDefinition(doc, operationName)
	if op == nil || op.Operation != ast.OperationTypeSubscription {
		return nil, fmt.Errorf("operation must be a subscription")
	}
	if op.SelectionSet == nil || len(op.SelectionSet.Selections) == 0 {
		return nil, fmt.Errorf("subscription must select a root field")
	}

	field, ok := op.SelectionSet.Selections[0].(*ast.Field)
	if !ok {
		return nil, fmt.Errorf("subscription root selection must be a field")
	}
	table := field.Name.Value
	if table == "" {
		return nil, fmt.Errorf("subscription table name is required")
	}

	where := map[string]any{}
	for _, arg := range field.Arguments {
		if arg == nil || arg.Name == nil || arg.Name.Value != "where" {
			continue
		}
		if value, ok := astValueToInterface(arg.Value, variables).(map[string]any); ok {
			where = value
		}
	}

	selection := map[string]*projectionField{}
	if field.SelectionSet != nil {
		selection = selectionSetToProjection(field.SelectionSet)
	}

	fieldName := table
	if field.Alias != nil && field.Alias.Value != "" {
		fieldName = field.Alias.Value
	}

	return &parsedSubscriptionRequest{
		table:     table,
		fieldName: fieldName,
		where:     where,
		selection: selection,
	}, nil
}

// astValueToInterface recursively converts a GraphQL AST value to a Go interface, handling objects, lists, variables, and scalar types.
func astValueToInterface(v ast.Value, variables map[string]any) any {
	switch node := v.(type) {
	case *ast.ObjectValue:
		out := make(map[string]any, len(node.Fields))
		for _, field := range node.Fields {
			out[field.Name.Value] = astValueToInterface(field.Value, variables)
		}
		return out
	case *ast.ListValue:
		out := make([]any, 0, len(node.Values))
		for _, item := range node.Values {
			out = append(out, astValueToInterface(item, variables))
		}
		return out
	case *ast.StringValue:
		return node.Value
	case *ast.BooleanValue:
		return node.Value
	case *ast.IntValue:
		if i, err := strconv.ParseInt(node.Value, 10, 64); err == nil {
			return i
		}
		return node.Value
	case *ast.FloatValue:
		if f, err := strconv.ParseFloat(node.Value, 64); err == nil {
			return f
		}
		return node.Value
	case *ast.EnumValue:
		return node.Value
	case *ast.Variable:
		if variables == nil {
			return nil
		}
		return variables[node.Name.Value]
	default:
		return nil
	}
}

// selectionSetToProjection converts a GraphQL selection set to a projection structure, mapping output field names to source fields and nested children.
func selectionSetToProjection(set *ast.SelectionSet) map[string]*projectionField {
	if set == nil {
		return nil
	}
	out := map[string]*projectionField{}
	for _, selection := range set.Selections {
		field, ok := selection.(*ast.Field)
		if !ok || field.Name == nil {
			continue
		}
		outputName := field.Name.Value
		if field.Alias != nil && field.Alias.Value != "" {
			outputName = field.Alias.Value
		}
		node := &projectionField{source: field.Name.Value}
		if field.SelectionSet == nil {
			out[outputName] = node
			continue
		}
		node.children = selectionSetToProjection(field.SelectionSet)
		out[outputName] = node
	}
	return out
}

func (h *Handler) storeSubscriptionState(connID, subID string, state *gqlwsSubscriptionState) {
	h.subMu.Lock()
	defer h.subMu.Unlock()
	if h.wsSubs == nil {
		h.wsSubs = make(map[string]map[string]*gqlwsSubscriptionState)
	}
	if h.wsSubs[connID] == nil {
		h.wsSubs[connID] = make(map[string]*gqlwsSubscriptionState)
	}
	h.wsSubs[connID][subID] = state
}

// unsubscribeWS removes a subscription from a WebSocket connection by ID, canceling its context and cleaning up hub resources.
func (h *Handler) unsubscribeWS(connID, subID string) {
	h.subMu.Lock()
	subs := h.wsSubs[connID]
	state := subs[subID]
	if state != nil {
		delete(subs, subID)
		if len(subs) == 0 {
			delete(h.wsSubs, connID)
		}
	}
	h.subMu.Unlock()

	if state == nil {
		return
	}
	state.cancel()
	if h.hub != nil {
		h.hub.Unsubscribe(state.hubClientID)
	}
}

func (h *Handler) unsubscribeAllWS(connID string) {
	h.subMu.Lock()
	subs := h.wsSubs[connID]
	delete(h.wsSubs, connID)
	h.subMu.Unlock()

	for _, state := range subs {
		state.cancel()
		if h.hub != nil {
			h.hub.Unsubscribe(state.hubClientID)
		}
	}
}

// forwardSubscriptionEvents forwards events from the realtime hub to a WebSocket connection, applying the subscription's where filter and field projection before sending.
func (h *Handler) forwardSubscriptionEvents(ctx context.Context, conn *GQLWSConn, subID string, client *realtime.Client, state *gqlwsSubscriptionState) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-client.Events():
			if !ok {
				return
			}
			if !h.canDeliverEvent(ctx, conn, event, state.where) {
				continue
			}
			payload := map[string]any{
				state.fieldName: projectRecord(eventRecordForDelivery(event), state.selection),
			}
			conn.SendNext(subID, payload)
		}
	}
}

func (h *Handler) canDeliverEvent(ctx context.Context, conn *GQLWSConn, event *realtime.Event, where map[string]any) bool {
	if event == nil {
		return false
	}
	if !realtime.CanSeeRecord(ctx, h.pool, h.cacheHolder, h.logger, conn.Claims(), event) {
		return false
	}
	row := eventRecordForDelivery(event)
	if row == nil {
		return false
	}
	return matchesGraphQLWhere(where, row)
}

func eventRecordForDelivery(event *realtime.Event) map[string]any {
	if event == nil {
		return nil
	}
	if event.Action == "delete" && event.OldRecord != nil {
		return event.OldRecord
	}
	return event.Record
}

// projectRecord filters a database record to include only selected fields based on the projection structure, supporting nested selections.
func projectRecord(row map[string]any, projection map[string]*projectionField) map[string]any {
	if row == nil {
		return nil
	}
	if len(projection) == 0 {
		out := make(map[string]any, len(row))
		for key, val := range row {
			out[key] = val
		}
		return out
	}

	out := make(map[string]any, len(projection))
	for key, child := range projection {
		raw, ok := row[child.source]
		if !ok {
			continue
		}
		if len(child.children) > 0 {
			rawMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			out[key] = projectRecord(rawMap, child.children)
			continue
		}
		out[key] = raw
	}
	return out
}
