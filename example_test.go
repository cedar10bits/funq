package funq_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/cedar10bits/funq"
)

// Sentinel errors for examples that turn an absent Optional into a failure.
var (
	errNoEven   = errors.New("no even element")
	errNotFound = errors.New("user not found")
)

func Example() {
	result := funq.FromFn(10, func(i int) int { return i + 1 }).
		Filter(func(v int) bool { return v%2 == 0 }). // Keep even numbers: [2, 4, 6, 8, 10]
		Map(func(v int) int { return v * 3 }).        // Multiply by 3: [6, 12, 18, 24, 30]
		Drop(1).                                      // Drop first: [12, 18, 24, 30]
		Take(3).                                      // Take first 3: [12, 18, 24]
		Slice()

	fmt.Println(result)
	// Output: [12 18 24]
}

func ExampleFromFn() {
	// FromFn creates elements on-demand (lazy evaluation): only the elements
	// the chain needs are generated.
	squares := funq.FromFn(1000000, func(i int) int { return i * i }).
		Filter(funq.LessThan(100)).
		Take(10).
		Slice()

	fmt.Println(squares)
	// Output: [0 1 4 9 16 25 36 49 64 81]
}

func ExampleFromSeq() {
	// FromSeq brings a stdlib iter.Seq into a Flow directly, so the rest of
	// the chain stays lazy. Without FromSeq, the only way in is materializing
	// the source first with slices.Collect.
	result := funq.FromSeq(slices.Values([]int{1, 2, 3, 4, 5})).
		Filter(func(v int) bool { return v%2 == 0 }).
		Map(func(v int) int { return v * 10 }).
		Slice()

	fmt.Println(result)
	// Output: [20 40]
}

func ExampleFlow_Cache() {
	// A chain's transforms may re-run on each terminal call, so a side-effecting
	// Map or Filter can fire more than once overall; either keep transforms pure
	// or use Cache to avoid repeated side effects.
	calls := 0
	eager := funq.From(1, 2, 3).Map(func(v int) int {
		calls++
		return v * 10
	})
	_ = eager.Slice()
	_ = eager.Slice()
	fmt.Println("From without Cache:", calls)

	calls = 0
	lazy := funq.FromFn(3, funq.Identity).Map(func(v int) int {
		calls++
		return v * 10
	})
	_ = lazy.Slice()
	_ = lazy.Slice()
	fmt.Println("without Cache:", calls)

	// Cache materializes the result once; later traversals reuse it instead
	// of re-running the chain.
	calls = 0
	cached := funq.FromFn(3, funq.Identity).Map(func(v int) int {
		calls++
		return v * 10
	}).Cache()
	_ = cached.Slice()
	_ = cached.Slice()
	fmt.Println("with Cache:", calls)
	// Output:
	// From without Cache: 6
	// without Cache: 6
	// with Cache: 3
}

func ExampleFlow_SortBy() {
	byLength := funq.From("ccc", "a", "bb").
		SortBy(func(s string) int { return len(s) }).
		Slice()

	fmt.Println(byLength)
	// Output: [a bb ccc]
}

func ExampleDistinct() {
	// Distinct keeps the first occurrence of each value.
	uniq := funq.Distinct(funq.From(1, 2, 2, 3, 1)).Slice()

	fmt.Println(uniq)
	// Output: [1 2 3]
}

func ExampleFlow_GroupBy() {
	groups := funq.From(1, 2, 3, 4, 5, 6).GroupBy(func(v int) int { return v % 2 })

	fmt.Println(groups[0])
	fmt.Println(groups[1])
	// Output:
	// [2 4 6]
	// [1 3 5]
}

func ExampleFlow_Concat() {
	combined := funq.From(1, 2).Concat(funq.From(3), funq.From(4, 5)).Slice()

	fmt.Println(combined)
	// Output: [1 2 3 4 5]
}

func ExampleZip() {
	// Zip pairs elements by position, stopping at the shorter Flow.
	pairs := funq.Zip(funq.From(1, 2, 3), funq.From("a", "b")).Slice()

	fmt.Println(pairs)
	// Output: [{1 a} {2 b}]
}

func ExampleFlow_ToMap() {
	// On a key collision the first occurrence wins.
	byLen := funq.From("a", "bb", "c").ToMap(func(s string) int { return len(s) })

	fmt.Println(byLen[1], byLen[2])
	// Output: a bb
}

func ExampleFlow_Partition() {
	even, odd := funq.From(1, 2, 3, 4, 5).Partition(func(v int) bool { return v%2 == 0 })

	fmt.Println(even.Slice())
	fmt.Println(odd.Slice())
	// Output:
	// [2 4]
	// [1 3 5]
}

func ExampleContains() {
	// Contains is a free function, not a method (see its doc comment for why).
	fmt.Println(funq.Contains(funq.From(1, 2, 3), 2))
	fmt.Println(funq.Contains(funq.From(1, 2, 3), 9))
	// Output:
	// true
	// false
}

func ExampleFlow_MinBy() {
	// Ties keep the first element.
	shortest := funq.From("ccc", "a", "bb").MinBy(func(s string) int { return len(s) })

	fmt.Println(shortest.MustGet())
	// Output: a
}

func ExampleChunk() {
	// Chunk cannot be a method (see its doc comment), so it is called
	// curried: Chunk(n)(flow), or in a chain via To: flow.To(Chunk[int](n)).
	// The final chunk may be shorter than n.
	chunks := funq.Chunk[int](2)(funq.From(1, 2, 3, 4, 5)).Slice()

	fmt.Println(chunks)
	// Output: [[1 2] [3 4] [5]]
}

func ExampleFlow_To() {
	// To applies a func(Flow[T]) U to the Flow itself, keeping free
	// functions like Chunk and Distinct in a fluent chain.
	sizes := funq.From(1, 2, 3, 4, 5).
		To(funq.Chunk[int](2)).
		Map(func(c []int) int { return len(c) }).
		Slice()
	sum := func(f funq.Flow[int]) int {
		return f.Reduce(func(a, b int) int { return a + b }).OrElse(0)
	}
	total := funq.From(1, 2, 2, 3).To(funq.Distinct).To(sum)

	fmt.Println(sizes, total)
	// Output: [2 2 1] 6
}

func ExampleSome() {
	// The chain stays Optional and can change type along the way.
	val := funq.Some(42).
		Filter(funq.GreaterThan(10)).
		Map(func(v int) int { return v * 2 }). // Optional[int]
		Map(strconv.Itoa)                      // Optional[string]

	fmt.Println(val.OrElse("none"))
	// Output: 84
}

func ExampleFromPtr() {
	ptr := &[]int{1, 2, 3}[0]
	opt := funq.FromPtr(ptr)

	fmt.Println(opt.OrElse(0))
	fmt.Println(funq.FromPtr[int](nil).OrElse(0))
	// Output:
	// 1
	// 0
}

func ExampleOptional_Ptr() {
	fmt.Println(*funq.Some(42).Ptr())
	fmt.Println(funq.None[int]().Ptr())
	// Output:
	// 42
	// <nil>
}

func ExampleFromResult() {
	// An error-returning call's results feed FromResult directly. The error is
	// dropped.
	fmt.Println(funq.FromResult(strconv.Atoi("42")).OrElse(-1))
	fmt.Println(funq.FromResult(strconv.Atoi("nope")).OrElse(-1))
	// Output:
	// 42
	// -1
}

func ExampleOptional_OrErr() {
	// OrErr leaves the fluent chain for Go's (value, error) idiom, so an
	// Optional-returning terminal can be returned as an ordinary failure.
	firstEven := func(xs ...int) (int, error) {
		return funq.From(xs...).Find(func(v int) bool { return v%2 == 0 }).
			OrErr(errNoEven)
	}

	fmt.Println(firstEven(1, 3, 4, 5))
	fmt.Println(firstEven(1, 3, 5))
	// Output:
	// 4 <nil>
	// 0 no even element
}

func ExampleOptional_AsFlow() {
	// AsFlow bridges into the full Flow API.
	nums := funq.Some(10).AsFlow().Map(func(v int) int { return v * 2 }).Slice()

	fmt.Println(nums)
	// Output: [20]
}

func ExampleOptional_MarshalJSON() {
	type Profile struct {
		Name  string                `json:"name"`
		Email funq.Optional[string] `json:"email"`          // null when None
		Phone funq.Optional[string] `json:"phone,omitzero"` // omitted when None
	}

	// Some(v) -> v, None -> null. Phone is None and dropped by omitzero.
	b, _ := json.Marshal(Profile{Name: "Ada", Email: funq.Some("ada@example.com")})
	fmt.Println(string(b))

	// On decode, an absent field or null becomes None.
	var p Profile
	_ = json.Unmarshal([]byte(`{"name":"Ada","email":null}`), &p)
	fmt.Println(p.Email.IsPresent(), p.Phone.IsPresent())
	// Output:
	// {"name":"Ada","email":"ada@example.com"}
	// false false
}

func ExampleGroove() {
	doubleOrError := func(n int) (int, error) {
		if n > 1000 {
			return 0, fmt.Errorf("number too large: %d", n)
		}
		return n * 2, nil
	}

	// Groove(f) cuts the first stage; each Jam appends exactly one more.
	parseDoubleStringify := funq.Groove(strconv.Atoi).
		Jam(doubleOrError).
		Jam(funq.NilError(strconv.Itoa))

	result, err := parseDoubleStringify.Play("21")
	fmt.Println(result, err)
	// Output: 42 <nil>
}

func ExampleErrOnNone() {
	// A lookup reports absence as None, not as an error.
	users := map[int]string{1: "ada"}
	lookup := func(id int) funq.Optional[string] {
		if name, ok := users[id]; ok {
			return funq.Some(name)
		}
		return funq.None[string]()
	}

	// ErrOnNone adapts it into a Groove stage without a wrapping closure.
	greet := funq.Groove(funq.ErrOnNone(lookup, errNotFound)).
		Jam(funq.NilError(func(name string) string { return "hello, " + name }))

	fmt.Println(greet.Play(1))
	_, err := greet.Play(2)
	fmt.Println(errors.Is(err, errNotFound))
	// Output:
	// hello, ada <nil>
	// true
}

func ExampleFlow_Filter() {
	// Predicates combine with And/Or/Not and plug into Filter.
	result := funq.From(-5, -2, 0, 3, 4, 7, 8, 12).
		Filter(funq.And(funq.GreaterThan(0), func(v int) bool { return v%2 == 0 })).
		Slice()

	fmt.Println(result)
	// Output: [4 8 12]
}

func ExampleFlow_Map() {
	doubled := funq.From(1, 2, 3, 4, 5).Map(func(v int) int { return v * 2 }).Slice()

	fmt.Println(doubled)
	// Output: [2 4 6 8 10]
}

func ExampleFlow_Reduce() {
	sum := funq.From(1, 2, 3, 4, 5).Reduce(func(a, b int) int { return a + b }).MustGet()

	fmt.Println(sum)
	// Output: 15
}

func ExampleCompose() {
	addThenFilter := func(n int) funq.Flow[int] {
		return funq.From(n).
			Map(func(v int) int { return v + 10 }).
			Filter(funq.GreaterThan(15))
	}

	// A no-arg method is already usable as a method expression
	// (funq.Flow[int].Slice is an Fp[Flow[int], []int]).
	pipeline := funq.Compose(addThenFilter).Then(funq.Flow[int].Slice)

	fmt.Println(pipeline.Run(5))
	fmt.Println(pipeline.Run(10))
	// Output:
	// []
	// [20]
}
