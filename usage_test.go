package funq_test

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/cedar10bits/funq"
)

// The README snippets are verified as Example functions in example_test.go.
// The tests below cover longer chains and edge cases kept out of the
// documentation.

func TestFlowChaining(t *testing.T) {
	t.Parallel()

	result := funq.FromFn(20, funq.Identity).
		Filter(func(y int) bool { return y%2 == 0 }).
		Map(func(y int) int { return y + 1 }).
		Filter(funq.GreaterThan(5)).
		Take(5).
		Reverse().
		Slice()

	if want := []int{15, 13, 11, 9, 7}; !reflect.DeepEqual(want, result) {
		t.Errorf("want %v, got %v", want, result)
	}
}

func TestOptionalChaining(t *testing.T) {
	t.Parallel()

	result := funq.Some(100).
		Filter(funq.GreaterThan(50)).
		Map(func(y int) int { return y / 10 }).
		Filter(func(y int) bool { return y%2 == 0 }).
		Map(func(y int) int { return y * 3 })

	if got := result.OrElse(0); got != 30 {
		t.Errorf("want 30, got %v", got)
	}

	noneResult := funq.Some(100).
		Filter(funq.GreaterThan(200)).
		Map(func(y int) int { return y / 10 })

	if got := noneResult.OrElse(0); got != 0 {
		t.Errorf("want 0, got %v", got)
	}
}

func TestPredicateCombinations(t *testing.T) {
	t.Parallel()

	isPositive := funq.GreaterThan(0)
	isLessThan10 := funq.LessThan(10)
	isValid := funq.And(isPositive, isLessThan10)

	if !isValid(5) {
		t.Error("want isValid(5) true, got false")
	}
	if isValid(0) {
		t.Error("want isValid(0) false, got true")
	}
	if isValid(10) {
		t.Error("want isValid(10) false, got true")
	}

	isEven := func(y int) bool { return y%2 == 0 }
	isSpecial := funq.Or(isEven, funq.GreaterThan(10))

	if !isSpecial(2) {
		t.Error("want isSpecial(2) true, got false")
	}
	if !isSpecial(11) {
		t.Error("want isSpecial(11) true, got false")
	}
	if isSpecial(3) {
		t.Error("want isSpecial(3) false, got true")
	}

	inRange := funq.Between(1, 10)
	if !inRange(1) {
		t.Error("want inRange(1) true, got false")
	}
	if !inRange(5) {
		t.Error("want inRange(5) true, got false")
	}
	if !inRange(10) {
		t.Error("want inRange(10) true, got false")
	}
	if inRange(0) {
		t.Error("want inRange(0) false, got true")
	}
	if inRange(11) {
		t.Error("want inRange(11) false, got true")
	}
}

func TestGrooveComposition(t *testing.T) {
	t.Parallel()

	add1 := func(y int) int { return y + 1 }
	mul2 := func(y int) int { return y * 2 }
	composed := funq.Compose(add1).Then(mul2)

	// Applies add1 then mul2: mul2(add1(x))
	if got := composed.Run(5); got != 12 {
		t.Errorf("want 12, got %v", got)
	}

	parseInt := func(s string) (int, error) {
		return strconv.Atoi(s)
	}
	double := func(n int) (int, error) {
		if n > 1000 {
			return 0, fmt.Errorf("number too large: %d", n)
		}
		return n * 2, nil
	}
	composeE := funq.Groove(parseInt).Jam(double)

	result, err := composeE.Play("5")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 10 {
		t.Errorf("want 10, got %v", result)
	}
}

// Element-wise min/max are spelled with MinBy/MaxBy and Identity rather
// than dedicated Min/Max methods. Membership is the free function Contains.
// See the MinBy and Contains doc comments for why the methods cannot exist.
// This test pins the idioms down so they keep compiling.
func TestElementWiseIdioms(t *testing.T) {
	t.Parallel()

	if got := funq.From(3, 1, 2).MinBy(funq.Identity).MustGet(); got != 1 {
		t.Errorf("want 1, got %v", got)
	}
	if got := funq.From(3, 1, 2).MaxBy(funq.Identity).MustGet(); got != 3 {
		t.Errorf("want 3, got %v", got)
	}
	if !funq.Contains(funq.From(1, 2, 3), 2) {
		t.Error("want Contains(..., 2) true, got false")
	}
	if funq.Contains(funq.From(1, 2, 3), 9) {
		t.Error("want Contains(..., 9) false, got true")
	}
}

func TestFromAndFromFn(t *testing.T) {
	t.Parallel()

	flow1 := funq.From(1, 2, 3, 4, 5)
	if want, got := []int{1, 2, 3, 4, 5}, flow1.Slice(); !reflect.DeepEqual(want, got) {
		t.Errorf("want %v, got %v", want, got)
	}
	if got := flow1.Count(); got != 5 {
		t.Errorf("want 5, got %v", got)
	}

	flow2 := funq.FromFn(5, func(y int) int { return y * 2 })
	if want, got := []int{0, 2, 4, 6, 8}, flow2.Slice(); !reflect.DeepEqual(want, got) {
		t.Errorf("want %v, got %v", want, got)
	}

	empty1 := funq.From[int]()
	if !empty1.IsEmpty() {
		t.Error("want empty1.IsEmpty() true, got false")
	}
	if got := empty1.Count(); got != 0 {
		t.Errorf("want 0, got %v", got)
	}

	empty2 := funq.FromFn(0, funq.Identity)
	if !empty2.IsEmpty() {
		t.Error("want empty2.IsEmpty() true, got false")
	}
}
