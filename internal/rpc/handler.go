package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

var (
	ctxType   = reflect.TypeFor[context.Context]()
	errorType = reflect.TypeFor[error]()
)

// ServiceHandler dispatches calls to svc's exported methods by name, the
// way Wails calls a bound service: a leading context.Context parameter
// gets the call's context, each JSON argument decodes into its parameter,
// a trailing error is the call's error, and the other results become null
// (none), the value (one) or a JSON array (several).
func ServiceHandler(svc any) Handler {
	v := reflect.ValueOf(svc)
	return func(ctx context.Context, method string, args []json.RawMessage) (json.RawMessage, error) {
		m := v.MethodByName(method)
		if !m.IsValid() {
			return nil, fmt.Errorf("rpc: unknown method %s", method)
		}
		t := m.Type()
		in := make([]reflect.Value, 0, t.NumIn())
		first := 0
		if t.NumIn() > 0 && t.In(0) == ctxType {
			in = append(in, reflect.ValueOf(ctx))
			first = 1
		}
		if want := t.NumIn() - first; len(args) != want {
			return nil, fmt.Errorf("rpc: %s takes %d arguments, got %d", method, want, len(args))
		}
		for i, raw := range args {
			p := reflect.New(t.In(first + i))
			if err := json.Unmarshal(raw, p.Interface()); err != nil {
				return nil, fmt.Errorf("rpc: %s argument %d: %w", method, i+1, err)
			}
			in = append(in, p.Elem())
		}
		out := m.Call(in)
		if n := len(out); n > 0 && t.Out(n-1) == errorType {
			if e := out[n-1]; !e.IsNil() {
				return nil, e.Interface().(error)
			}
			out = out[:n-1]
		}
		switch len(out) {
		case 0:
			return json.RawMessage("null"), nil
		case 1:
			return json.Marshal(out[0].Interface())
		}
		vals := make([]any, len(out))
		for i, o := range out {
			vals[i] = o.Interface()
		}
		return json.Marshal(vals)
	}
}
