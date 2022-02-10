package lookup

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestLookup_Map(c *C) {
	value, err := Lookup(map[string]int{"foo": 42}, "foo", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 42)
}

func (s *S) TestLookup_Ptr(c *C) {
	value, err := Lookup(&structFixture, "String", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "foo")
}

func (s *S) TestLookup_Interface(c *C) {
	value, err := Lookup(structFixture, "Interface", Options{})

	c.Assert(err, IsNil)
	c.Assert(value, Equals, "foo")
}

func (s *S) TestLookup_StructBasic(c *C) {
	value, err := Lookup(structFixture, "String", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "foo")
}

func (s *S) TestLookup_StructPlusMap(c *C) {
	value, err := Lookup(structFixture, "Map.foo", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 42)
}

func (s *S) TestLookup_MapNamed(c *C) {
	value, err := Lookup(mapFixtureNamed, "foo", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 42)
}

func (s *S) TestLookup_NotFound(c *C) {
	_, err := Lookup(structFixture, "qux", Options{})
	c.Assert(err, Equals, ErrKeyNotFound)

	_, err = Lookup(mapFixture, "qux", Options{})
	c.Assert(err, Equals, ErrKeyNotFound)
}

func (s *S) TestAggregableLookup_StructIndex(c *C) {
	value, err := Lookup(structFixture, "StructSlice.Map.foo", Options{})

	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, []int{42, 42})
}

func (s *S) TestAggregableLookup_StructNestedMap(c *C) {
	value, err := Lookup(structFixture, "StructSlice[0].String", Options{})

	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, "foo")
}

func (s *S) TestAggregableLookup_StructNested(c *C) {
	value, err := Lookup(structFixture, "StructSlice.StructSlice.String", Options{})

	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, []string{"bar", "foo", "qux", "baz"})
}

func (s *S) TestAggregableLookupString_Complex(c *C) {
	value, err := Lookup(structFixture, "StructSlice.StructSlice[0].String", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, []string{"bar", "foo", "qux", "baz"})

	value, err = Lookup(structFixture, "StructSlice[0].Map.foo", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, 42)

	value, err = Lookup(mapComplexFixture, "map.bar", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, 1)

	value, err = Lookup(mapComplexFixture, "list.baz", Options{})
	c.Assert(err, IsNil)
	c.Assert(value, DeepEquals, []int{1, 2, 3})
}

func (s *S) TestAggregableLookup_EmptySlice(c *C) {
	fixture := [][]MyStruct{{}}
	value, err := Lookup(fixture, "String", Options{})
	c.Assert(err, IsNil)
	c.Assert(value.([]string), DeepEquals, []string{})
}

func (s *S) TestAggregableLookup_EmptyMap(c *C) {
	fixture := map[string]*MyStruct{}
	value, err := Lookup(fixture, "Map", Options{})
	c.Assert(err, IsNil)
	c.Assert(value.([]map[string]int), DeepEquals, []map[string]int{})
}

func (s *S) TestMergeValue(c *C) {
	v := mergeValue([]reflect.Value{reflect.ValueOf("qux"), reflect.ValueOf("foo")})
	c.Assert(v.Interface(), DeepEquals, []string{"qux", "foo"})
}

func (s *S) TestMergeValueSlice(c *C) {
	v := mergeValue([]reflect.Value{
		reflect.ValueOf([]string{"foo", "bar"}),
		reflect.ValueOf([]string{"qux", "baz"}),
	})

	c.Assert(v.Interface(), DeepEquals, []string{"foo", "bar", "qux", "baz"})
}

func (s *S) TestMergeValueZero(c *C) {
	v := mergeValue([]reflect.Value{reflect.Value{}, reflect.ValueOf("foo")})
	c.Assert(v.Interface(), DeepEquals, []string{"foo"})
}

func (s *S) TestParseIndex(c *C) {
	key, index, err := parseIndex("foo[42]")
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "foo")
	c.Assert(index, Equals, 42)
}

func (s *S) TestParseIndexNooIndex(c *C) {
	key, index, err := parseIndex("foo")
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "foo")
	c.Assert(index, Equals, -1)
}

func (s *S) TestParseIndexMalFormed(c *C) {
	key, index, err := parseIndex("foo[]")
	c.Assert(err, Equals, ErrMalformedIndex)
	c.Assert(key, Equals, "")
	c.Assert(index, Equals, -1)

	key, index, err = parseIndex("foo[42")
	c.Assert(err, Equals, ErrMalformedIndex)
	c.Assert(key, Equals, "")
	c.Assert(index, Equals, -1)

	key, index, err = parseIndex("foo42]")
	c.Assert(err, Equals, ErrMalformedIndex)
	c.Assert(key, Equals, "")
	c.Assert(index, Equals, -1)
}

func (s *S) TestLookup_CaseSensitive(c *C) {
	_, err := Lookup(structFixture, "STring", Options{})
	c.Assert(err, Equals, ErrKeyNotFound)
}

func (s *S) TestLookup_CaseInsensitive(c *C) {
	value, err := Lookup(structFixture, "STring", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "foo")
}

func (s *S) TestLookup_CaseInsensitive_ExactMatch(c *C) {
	value, err := Lookup(caseFixtureStruct, "Testfield", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 2)
}

func (s *S) TestLookup_CaseInsensitive_FirstMatch(c *C) {
	value, err := Lookup(caseFixtureStruct, "testfield", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 1)
}

func (s *S) TestLookup_CaseInsensitiveExactMatch(c *C) {
	value, err := Lookup(structFixture, "STring", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "foo")
}

func (s *S) TestLookup_Map_CaseSensitive(c *C) {
	_, err := Lookup(map[string]int{"Foo": 42}, "foo", Options{})
	c.Assert(err, Equals, ErrKeyNotFound)
}

func (s *S) TestLookup_Map_CaseInsensitive(c *C) {
	value, err := Lookup(map[string]int{"Foo": 42}, "foo", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 42)
}

func (s *S) TestLookup_Map_CaseInsensitive_ExactMatch(c *C) {
	value, err := Lookup(caseFixtureMap, "Testkey", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, 2)
}

func (s *S) TestLookup_ListPtr(c *C) {
	type Inner struct {
		Value string
	}

	type Outer struct {
		Values *[]Inner
	}

	values := []Inner{{Value: "first"}, {Value: "second"}}
	data := Outer{Values: &values}

	value, err := Lookup(data, "Values[0].Value", Options{CaseInsentitive: true})
	c.Assert(err, IsNil)
	c.Assert(value, Equals, "first")
}

func TestLookup(t *testing.T) {
	testCases := []struct {
		desc    string
		input   interface{}
		path    string
		opts    Options
		want    interface{}
		wantErr codes.Code
	}{
		{
			desc:  "Direct Access",
			input: structFixture.JSONString,
			path:  "String",
			opts: Options{
				ExpandStringAsJSON: true,
			},
			want: "Abc",
		},
		{
			desc:  "Field of Struct",
			input: structFixture.JSONString,
			path:  "Struct.Substring",
			opts: Options{
				ExpandStringAsJSON: true,
			},
			want: "Abcd",
		},
		{
			desc:  "Array",
			input: structFixture.JSONString,
			path:  "Struct.Array[1]",
			opts: Options{
				ExpandStringAsJSON: true,
			},
			want: float64(2),
		},
		{
			desc:  "Expanded String - Direct Access",
			input: structFixture,
			path:  "JSONString.String",
			opts: Options{
				ExpandStringAsJSON: true,
			},
			want: "Abc",
		},
		{
			desc:  "Expanded String - Field of Struct",
			input: structFixture,
			path:  "JSONString.Struct.Substring",
			opts: Options{
				ExpandStringAsJSON: true,
			},
			want: "Abcd",
		},
		{
			desc:  "Expanded String - Array",
			input: structFixture,
			path:  "JSONString.Struct.Array[1]",
			opts: Options{
				ExpandStringAsJSON: true,
			},
			want: float64(2),
		},
	}

	for _, tc := range testCases {
		// Test for both case sensitive and case insensitive.
		for _, caseSensitive := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s - CaseSensitive=%v", tc.desc, caseSensitive), func(t *testing.T) {
				opts := tc.opts
				opts.CaseInsentitive = caseSensitive

				got, err := Lookup(tc.input, tc.path, opts)
				if code := status.Code(err); code != tc.wantErr {
					t.Fatalf("Lookup() returned error %s(%v), want %s", code, err, tc.wantErr)
				}
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("Lookup() returned unexpected value. diff: (-want +got)\n%s", diff)
				}
			})
		}
	}
}

func ExampleLookupString() {
	type Cast struct {
		Actor, Role string
	}

	type Serie struct {
		Cast []Cast
	}

	series := map[string]Serie{
		"A-Team": {Cast: []Cast{
			{Actor: "George Peppard", Role: "Hannibal"},
			{Actor: "Dwight Schultz", Role: "Murdock"},
			{Actor: "Mr. T", Role: "Baracus"},
			{Actor: "Dirk Benedict", Role: "Faceman"},
		}},
	}

	q := "A-Team.Cast.Role"
	value, _ := Lookup(series, q, Options{})
	fmt.Println(q, "->", value)

	q = "A-Team.Cast[0].Actor"
	value, _ = Lookup(series, q, Options{})
	fmt.Println(q, "->", value)

	// Output:
	// A-Team.Cast.Role -> [Hannibal Murdock Baracus Faceman]
	// A-Team.Cast[0].Actor -> George Peppard
}

func ExampleLookup() {
	type ExampleStruct struct {
		Values struct {
			Foo int
		}
	}

	i := ExampleStruct{}
	i.Values.Foo = 10

	value, _ := Lookup(i, "Values.Foo", Options{})
	fmt.Println(value)
	// Output: 10
}

func ExampleCaseInsensitive() {
	type ExampleStruct struct {
		SoftwareUpdated bool
	}

	i := ExampleStruct{
		SoftwareUpdated: true,
	}

	value, _ := Lookup(i, "softwareupdated", Options{CaseInsentitive: true})
	fmt.Println(value)
	// Output: true
}

type MyStruct struct {
	String      string
	Map         map[string]int
	Nested      *MyStruct
	StructSlice []*MyStruct
	Interface   interface{}
	JSONString  string
}

type MyKey string

var mapFixtureNamed = map[MyKey]int{"foo": 42}
var mapFixture = map[string]int{"foo": 42}
var structFixture = MyStruct{
	String:    "foo",
	Map:       mapFixture,
	Interface: "foo",
	StructSlice: []*MyStruct{
		{Map: mapFixture, String: "foo", StructSlice: []*MyStruct{{String: "bar"}, {String: "foo"}}},
		{Map: mapFixture, String: "qux", StructSlice: []*MyStruct{{String: "qux"}, {String: "baz"}}},
	},
	JSONString: `
	{
		"String": "Abc",
		"Struct": {
			"Substring": "Abcd",
			"Array": [1, 2, 3],
			"ArrayInArray": [
				[1, 2, 3],
				[4, 5, 6]
			],
			"StructInArray": [
				{
					"FieldA": "Abc",
					"FieldB": 123
				},
				{
					"Field1": "abc",
					"Field2": 123
				}
			]
		}
	}`,
}

var mapComplexFixture = map[string]interface{}{
	"map": map[string]interface{}{
		"bar": 1,
	},
	"list": []map[string]interface{}{
		{"baz": 1},
		{"baz": 2},
		{"baz": 3},
	},
}

var caseFixtureStruct = struct {
	Foo       int
	TestField int
	Testfield int
	testField int
}{
	0, 1, 2, 3,
}

var caseFixtureMap = map[string]int{
	"Foo":     0,
	"TestKey": 1,
	"Testkey": 2,
	"testKey": 3,
}
