// Package reflectutil provides shared reflection helpers.
package reflectutil

import "reflect"

// IsNil reports whether value is nil or holds a nil pointer, map, slice,
// channel, or function. Other values return false.
func IsNil(value any) bool {
	if value == nil {
		return true
	}
	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
