package funq

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
)

func TestFromPtr(t *testing.T) {
	t.Parallel()
	ten := 10
	assertEqual(t, None[int](), FromPtr[int](nil))
	assertEqual(t, Some(10), FromPtr(&ten))
}

func TestFromResult(t *testing.T) {
	t.Parallel()
	assertEqual(t, Some(42), FromResult(42, nil))
	assertEqual(t, None[int](), FromResult(42, errSentinel))
	assertEqual(t, Some(42), FromResult(strconv.Atoi("42")))
	assertEqual(t, None[int](), FromResult(strconv.Atoi("nope")))
}

func TestOptionalOrErr(t *testing.T) {
	t.Parallel()
	v, err := Some(1).OrErr(errSentinel)
	assertEqual(t, 1, v)
	mustNoErr(t, err)

	v, err = None[int]().OrErr(errSentinel)
	assertEqual(t, 0, v)
	if !errors.Is(err, errSentinel) {
		t.Errorf("want errors.Is(%v, %v), got false", err, errSentinel)
	}
}

func TestOptionalOrErrRoundTrip(t *testing.T) {
	t.Parallel()
	// FromResult drops the original error, so a round trip preserves only
	// presence: what comes back is whichever error OrErr was handed.
	_, err := FromResult(strconv.Atoi("nope")).OrErr(errSentinel)
	if !errors.Is(err, errSentinel) {
		t.Errorf("want errors.Is(%v, %v), got false", err, errSentinel)
	}
	var numErr *strconv.NumError
	isFalse(t, errors.As(err, &numErr))
}

func TestZeroValueOptionalIsNone(t *testing.T) {
	t.Parallel()
	var o Optional[int]
	isTrue(t, o.IsEmpty())
	isFalse(t, o.IsPresent())
}

func TestOptionalGet(t *testing.T) {
	t.Parallel()
	v, ok := Some(1).Get()
	assertEqual(t, 1, v)
	isTrue(t, ok)

	_, ok = None[int]().Get()
	isFalse(t, ok)
}

func TestOptionalOrElse(t *testing.T) {
	t.Parallel()
	assertEqual(t, 100, None[int]().OrElse(100))
	assertEqual(t, 1, Some(1).OrElse(100))
}

func TestOptionalOrElseGet(t *testing.T) {
	t.Parallel()
	assertEqual(t, 100, None[int]().OrElseGet(func() int { return 100 }))
	assertEqual(t, 1, Some(1).OrElseGet(func() int { return 100 }))
}

func TestOptionalMustGet(t *testing.T) {
	t.Parallel()
	if _, panicked := didPanic(func() { _ = None[int]().MustGet() }); !panicked {
		t.Errorf("want fn to panic, it did not")
	}
	assertEqual(t, 1, Some(1).MustGet())
}

func TestOptionalOr(t *testing.T) {
	t.Parallel()
	assertEqual(t, Some(1), Some(1).Or(Some(2)))
	assertEqual(t, Some(2), None[int]().Or(Some(2)))
	assertEqual(t, None[int](), None[int]().Or(None[int]()))
}

func TestOptionalFilter(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), None[int]().Filter(True))
	assertEqual(t, Some(1), Some(1).Filter(Equal(1)))
	assertEqual(t, None[int](), Some(1).Filter(Equal(2)))
}

func TestOptionalMapSameType(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), None[int]().Map(mul2))
	assertEqual(t, Some(2), Some(1).Map(mul2))
}

func TestOptionalMapTypeChanging(t *testing.T) {
	t.Parallel()
	got := Some(21).
		Filter(GreaterThan(10)).
		Map(mul2).
		Map(strconv.Itoa).
		OrElse("none")
	assertEqual(t, "42", got)

	got2 := Some(5).
		Filter(GreaterThan(10)).
		Map(strconv.Itoa).
		OrElse("none")
	assertEqual(t, "none", got2)
}

func TestOptionalFlatMap(t *testing.T) {
	t.Parallel()
	half := func(n int) Optional[int] {
		if n%2 == 0 {
			return Some(n / 2)
		}
		return None[int]()
	}
	assertEqual(t, Some(2), Some(4).FlatMap(half))
	assertEqual(t, None[int](), Some(3).FlatMap(half))
	assertEqual(t, None[int](), None[int]().FlatMap(half))
}

func TestOptionalForEach(t *testing.T) {
	t.Parallel()
	got := []int{}
	Some(1).ForEach(func(n int) { got = append(got, n) })
	None[int]().ForEach(func(n int) { got = append(got, n) })
	assertEqual(t, []int{1}, got)
}

func TestOptionalSeq(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		val  Optional[int]
		want []int
	}{
		{"Some", Some(42), []int{42}},
		{"None", None[int](), []int{}},
		{"Matched", Some(4).Filter(even), []int{4}},
		{"Unmatched", Some(3).Filter(even), []int{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := []int{}
			for v := range tc.val.Seq() {
				got = append(got, v)
			}
			assertEqual(t, tc.want, got)
		})
	}
}

func TestOptionalAsFlow(t *testing.T) {
	t.Parallel()
	assertEqual(t, []int{42}, Some(42).AsFlow().Slice())
	assertEqual(t, []int{}, None[int]().AsFlow().Slice())
	got := Some(10).AsFlow().Map(mul2).Slice()
	assertEqual(t, []int{20}, got)
}

func TestOptionalString(t *testing.T) {
	t.Parallel()
	assertEqual(t, "Some(42)", Some(42).String())
	assertEqual(t, "Some(hi)", Some("hi").String())
	assertEqual(t, "None", None[int]().String())
	assertEqual(t, "Some(42)", fmt.Sprintf("%v", Some(42)))
}

func TestOptionalMarshalJSON(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(Some(42))
	mustNoErr(t, err)
	assertEqual(t, "42", string(b))

	b, err = json.Marshal(Some("hi"))
	mustNoErr(t, err)
	assertEqual(t, `"hi"`, string(b))

	b, err = json.Marshal(None[int]())
	mustNoErr(t, err)
	assertEqual(t, "null", string(b))
}

func TestOptionalUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var some Optional[int]
	mustNoErr(t, json.Unmarshal([]byte("42"), &some))
	assertEqual(t, Some(42), some)

	var null Optional[int]
	mustNoErr(t, json.Unmarshal([]byte("null"), &null))
	assertEqual(t, None[int](), null)

	// decode error from the inner type propagates
	var bad Optional[int]
	if err := json.Unmarshal([]byte(`"nope"`), &bad); err == nil {
		t.Errorf("want an error, got nil")
	}
}

func TestOptionalJSONRoundTripInStruct(t *testing.T) {
	t.Parallel()
	type doc struct {
		A Optional[int]    `json:"a"`
		B Optional[string] `json:"b"`
		C Optional[int]    `json:"c,omitzero"`
	}

	in := doc{A: Some(1), B: None[string]()}
	b, err := json.Marshal(in)
	mustNoErr(t, err)
	// B (None) serializes as null. C (None) is dropped by omitzero.
	assertEqual(t, `{"a":1,"b":null}`, string(b))

	// Absent field and explicit null both decode to None.
	var out doc
	mustNoErr(t, json.Unmarshal([]byte(`{"a":7,"b":null}`), &out))
	assertEqual(t, Some(7), out.A)
	assertEqual(t, None[string](), out.B)
	assertEqual(t, None[int](), out.C)
}

func TestOptionalIsZero(t *testing.T) {
	t.Parallel()
	isTrue(t, None[int]().IsZero())
	isFalse(t, Some(0).IsZero())
}

func TestOptionalComposedWithCompose(t *testing.T) {
	t.Parallel()
	parseThenDouble := Compose(func(o Optional[int]) Optional[string] {
		return o.Map(strconv.Itoa)
	}).Then(func(o Optional[string]) string {
		return o.OrElse("none")
	})
	assertEqual(t, "2", parseThenDouble.Run(Some(2)))
	assertEqual(t, "none", parseThenDouble.Run(None[int]()))
}
