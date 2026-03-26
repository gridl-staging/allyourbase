package edgefunc

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
)

// EmailSendFunc handles ayb.email.send() calls from edge functions.
// Returns count of successfully sent emails.
type EmailSendFunc func(ctx context.Context, to []string, subject, html, text, templateKey string, variables map[string]string, from string) (int, error)

// registerEmailBridge adds ayb.email.send(opts) to the given ayb object.
// No-op (returns nil) when sendFn is nil.
func registerEmailBridge(
	vm *goja.Runtime,
	ctx context.Context,
	ayb *goja.Object,
	sendFn EmailSendFunc,
) error {
	if sendFn == nil {
		return nil
	}

	emailObj := vm.NewObject()
	_ = emailObj.Set("send", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("ayb.email.send() requires an options object"))
		}
		exported, ok := call.Arguments[0].Export().(map[string]interface{})
		if !ok {
			panic(vm.NewTypeError("ayb.email.send() argument must be an object"))
		}

		// Extract "to" — string or array of strings.
		to, err := extractEmailTo(exported)
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		if len(to) == 0 {
			panic(vm.NewTypeError("ayb.email.send(): 'to' is required"))
		}

		subject, _ := exported["subject"].(string)
		html, _ := exported["html"].(string)
		text, _ := exported["text"].(string)
		templateKey, _ := exported["templateKey"].(string)
		from, _ := exported["from"].(string)

		var variables map[string]string
		if raw, ok := exported["variables"]; ok {
			if m, ok := raw.(map[string]interface{}); ok {
				variables = make(map[string]string, len(m))
				for k, v := range m {
					variables[k] = fmt.Sprintf("%v", v)
				}
			}
		}

		sent, err := sendFn(ctx, to, subject, html, text, templateKey, variables, from)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("ayb.email.send: %w", err)))
		}

		result := vm.NewObject()
		_ = result.Set("sent", sent)
		return result
	})

	return ayb.Set("email", emailObj)
}

// extractEmailTo extracts the "to" field from a JS options object.
// Accepts string or []interface{} (array of strings from JS).
func extractEmailTo(opts map[string]interface{}) ([]string, error) {
	raw, ok := opts["to"]
	if !ok {
		return nil, nil
	}

	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		return []string{v}, nil
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("ayb.email.send(): 'to' array elements must be strings")
			}
			if s != "" {
				result = append(result, s)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("ayb.email.send(): 'to' must be a string or array of strings")
	}
}
