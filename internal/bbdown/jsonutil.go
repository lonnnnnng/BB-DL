package bbdown

import (
	"encoding/json"
	"fmt"
)

type J map[string]any

func ParseJ(data []byte) (J, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("json root is not object")
	}
	return J(obj), nil
}

func (j J) Obj(key string) J {
	if j == nil {
		return nil
	}
	if v, ok := j[key].(map[string]any); ok {
		return J(v)
	}
	return nil
}

func (j J) Arr(key string) []any {
	if j == nil {
		return nil
	}
	if v, ok := j[key].([]any); ok {
		return v
	}
	return nil
}

func (j J) Str(key string) string {
	if j == nil {
		return ""
	}
	switch v := j[key].(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return ""
	}
}

func (j J) Int64(key string) int64 {
	if j == nil {
		return 0
	}
	switch v := j[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		var n int64
		_, _ = fmt.Sscan(v, &n)
		return n
	default:
		return 0
	}
}

func (j J) Int(key string) int {
	return int(j.Int64(key))
}

func (j J) Bool(key string) bool {
	if j == nil {
		return false
	}
	if v, ok := j[key].(bool); ok {
		return v
	}
	return false
}

func asJ(v any) J {
	if m, ok := v.(map[string]any); ok {
		return J(m)
	}
	return nil
}

func arrItems(arr []any) []J {
	out := make([]J, 0, len(arr))
	for _, v := range arr {
		if obj := asJ(v); obj != nil {
			out = append(out, obj)
		}
	}
	return out
}

