/*
Small library on top of reflect for make lookups to Structs or Maps. Using a
very simple DSL you can access to any property, key or value of any value of Go.
*/
package lookup

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultSplitToken = "."
	indexCloseChar    = "]"
	indexOpenChar     = "["
)

type MatchFunc func(string) string

type Options struct {
	// If true, any string that can be parsed into JSON will be expanded as map[string]interface{}
	ExpandStringAsJSON bool
	// A list of functions to be applied before compaing the path and field name.
	// A section of path and a field in the struct match if any of MatchFunctions returns the same string.
	// i.e. matchFunc(path) == matchFunc(field)
	MatchFunctions []MatchFunc
	// The token used to split a path. If not specified, by default it's ".".
	SplitToken string
}

// LookupString performs a lookup into a value, using a string. Same as `Lookup`
// but using a string with the keys separated by `.`
// Lookup performs a lookup into a value, using a path of keys. The key should
// match with a Field or a MapIndex. For slice you can use the syntax key[index]
// to access a specific index. If one key owns to a slice and an index is not
// specificied the rest of the path will be apllied to evaley value of the
// slice, and the value will be merged into a slice.
func Lookup(i interface{}, path string, opts Options) (interface{}, error) {
	v, err := lookup(i, strings.Split(path, getSplitToken(&opts)), opts)
	if err == nil {
		return v.Interface(), nil
	}
	return nil, err
}

func lookup(i interface{}, path []string, opts Options) (reflect.Value, error) {
	value := reflect.ValueOf(i)
	var parent reflect.Value
	var err error

	for i, part := range path {
		if opts.ExpandStringAsJSON {
			// Expand the value if it's expandable and not the last value.
			if out := expandStringAsJSON(value); out != nil {
				value = reflect.ValueOf(out)
			}
		}
		parent = value

		value, err = getValueByName(value, part, opts)
		if err == nil {
			continue
		}

		if !isAggregable(parent) {
			break
		}

		value, err = aggreateAggregableValue(parent, path[i:], opts)
		break
	}

	return value, err
}

func getValueByName(v reflect.Value, key string, opts Options) (reflect.Value, error) {
	var value reflect.Value
	var index int = -1
	var err error

	key, index, err = parseIndex(key)
	if err != nil {
		return reflect.Value{}, err
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return getValueByName(v.Elem(), key, opts)
	case reflect.Struct:
		value = v.FieldByName(key)

		if value.Kind() == reflect.Invalid {
			// We don't use FieldByNameFunc, since it returns zero value if the
			// match func matches multiple fields. Iterate here and return the
			// first matching field.
			for i := 0; i < v.NumField(); i++ {
				if compareWithMatchFunc(opts.MatchFunctions, v.Type().Field(i).Name, key) {
					value = v.Field(i)
					break
				}
			}
		}

	case reflect.Map:
		kValue := reflect.Indirect(reflect.New(v.Type().Key()))
		kValue.SetString(key)
		value = v.MapIndex(kValue)
		if value.Kind() == reflect.Invalid {
			iter := v.MapRange()
			for iter.Next() {
				if compareWithMatchFunc(opts.MatchFunctions, key, iter.Key().String()) {
					kValue.SetString(iter.Key().String())
					value = v.MapIndex(kValue)
					break
				}
			}
		}
	}

	if !value.IsValid() {
		return reflect.Value{}, status.Errorf(codes.NotFound, "key %q not found", key)
	}

	value = getRealValue(value)
	if index != -1 {
		if value.Type().Kind() != reflect.Slice {
			return reflect.Value{}, status.Errorf(codes.InvalidArgument, "key %q is not a list", key)
		}

		value = getRealValue(value.Index(index))
	}

	return value, nil
}

func getRealValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) {
		v = v.Elem()
	}
	return v
}

func aggreateAggregableValue(v reflect.Value, path []string, opts Options) (reflect.Value, error) {
	values := make([]reflect.Value, 0)

	l := v.Len()
	if l == 0 {
		ty, ok := lookupType(v.Type(), path...)
		if !ok {
			return reflect.Value{}, status.Errorf(codes.NotFound, "path %q not found", strings.Join(path, getSplitToken(&opts)))
		}
		return reflect.MakeSlice(reflect.SliceOf(ty), 0, 0), nil
	}

	index := indexFunction(v)
	for i := 0; i < l; i++ {
		value, err := lookup(index(i).Interface(), path, opts)
		if err != nil {
			return reflect.Value{}, err
		}

		values = append(values, value)
	}

	return mergeValue(values), nil
}

func indexFunction(v reflect.Value) func(i int) reflect.Value {
	switch v.Kind() {
	case reflect.Slice:
		return v.Index
	case reflect.Map:
		keys := v.MapKeys()
		return func(i int) reflect.Value {
			return v.MapIndex(keys[i])
		}
	default:
		panic("unsuported kind for index")
	}
}

func mergeValue(values []reflect.Value) reflect.Value {
	values = removeZeroValues(values)
	l := len(values)
	if l == 0 {
		return reflect.Value{}
	}

	sample := values[0]
	mergeable := isMergeable(sample)

	t := sample.Type()
	if mergeable {
		t = t.Elem()
	}

	value := reflect.MakeSlice(reflect.SliceOf(t), 0, 0)
	for i := 0; i < l; i++ {
		if !values[i].IsValid() {
			continue
		}

		if mergeable {
			value = reflect.AppendSlice(value, values[i])
		} else {
			value = reflect.Append(value, values[i])
		}
	}

	return value
}

func removeZeroValues(values []reflect.Value) []reflect.Value {
	l := len(values)

	var v []reflect.Value
	for i := 0; i < l; i++ {
		if values[i].IsValid() {
			v = append(v, values[i])
		}
	}

	return v
}

func isAggregable(v reflect.Value) bool {
	k := v.Kind()

	return k == reflect.Map || k == reflect.Slice
}

func isMergeable(v reflect.Value) bool {
	k := v.Kind()
	return k == reflect.Map || k == reflect.Slice
}

func parseIndex(s string) (string, int, error) {
	start := strings.Index(s, indexOpenChar)
	end := strings.Index(s, indexCloseChar)

	if start == -1 && end == -1 {
		return s, -1, nil
	}

	if (start != -1 && end == -1) || (start == -1 && end != -1) {
		return "", -1, status.Errorf(codes.InvalidArgument, "invalid index %q", s)
	}

	index, err := strconv.Atoi(s[start+1 : end])
	if err != nil {
		return "", -1, status.Errorf(codes.InvalidArgument, "invalid index %q", s)
	}

	return s[:start], index, nil
}

func lookupType(ty reflect.Type, path ...string) (reflect.Type, bool) {
	if len(path) == 0 {
		return ty, true
	}

	switch ty.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		if strings.ContainsAny(path[0], indexOpenChar+indexCloseChar) {
			return lookupType(ty.Elem(), path[1:]...)
		}
		// Aggregate.
		return lookupType(ty.Elem(), path...)
	case reflect.Ptr:
		return lookupType(ty.Elem(), path...)
	case reflect.Interface:
		// We can't know from here without a value. Let's just return this type.
		return ty, true
	case reflect.Struct:
		f, ok := ty.FieldByName(path[0])
		if ok {
			return lookupType(f.Type, path[1:]...)
		}
	}
	return nil, false
}

// If the input value is expandable as JSON, returns a non-nil map.
func expandStringAsJSON(v reflect.Value) map[string]interface{} {
	if v.Kind() != reflect.String || !v.IsValid() || v.IsZero() {
		return nil
	}
	jsonValue := make(map[string]interface{})
	// Only returns the JSON instance when marshal succeeds.
	if err := json.Unmarshal([]byte(v.String()), &jsonValue); err == nil {
		return jsonValue
	}
	return nil
}

func getSplitToken(opts *Options) string {
	if opts != nil && opts.SplitToken != "" {
		return opts.SplitToken
	}
	return defaultSplitToken
}

func compareWithMatchFunc(matchFuncs []MatchFunc, a, b string) bool {
	for _, f := range matchFuncs {
		if f(a) == f(b) {
			return true
		}
	}
	return false
}
