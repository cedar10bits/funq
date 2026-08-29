package funq

import (
	"cmp"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Helpers shared across this package's test files. The assertion helpers
// below stand in for testify: the plain ones for assert, mustNoErr for
// require.

// msgSuffix appends the caller's optional note. Taking a string rather than
// testify's (format, args...) keeps these helpers from looking like print
// wrappers to vet, which would otherwise flag their format strings. A caller
// needing formatting passes fmt.Sprintf, where vet still checks it.
func msgSuffix(msg []string) string {
	if len(msg) == 0 {
		return ""
	}
	return ": " + strings.Join(msg, " ")
}

func assertEqual[T any](t *testing.T, want, got T, msg ...string) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want %#v, got %#v%s", want, got, msgSuffix(msg))
	}
}

func isTrue(t *testing.T, got bool, msg ...string) {
	t.Helper()
	if !got {
		t.Errorf("want true, got false%s", msgSuffix(msg))
	}
}

func isFalse(t *testing.T, got bool, msg ...string) {
	t.Helper()
	if got {
		t.Errorf("want false, got true%s", msgSuffix(msg))
	}
}

func didPanic(fn func()) (val any, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			val, panicked = r, true
		}
	}()
	fn()
	return
}

func mustNoErr(t *testing.T, err error, msg ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v%s", err, msgSuffix(msg))
	}
}

// errSentinel stands in wherever a test needs only an error's identity, so
// errors.Is can confirm the exact value survived.
var errSentinel = errors.New("sentinel")

// kv pairs a sort key with a distinct tag, so a test can tell which of several
// equal-keyed elements it is looking at and thereby observe sort stability.
type kv struct {
	k int
	v string
}

// sortByComparator is the comparator formulation [Flow.SortBy] used before it
// moved to sorting a permutation of indices: key travels into
// slices.SortStableFunc and is therefore evaluated on every comparison. It is
// the baseline both TestSortByMatchesComparatorSort and
// BenchmarkSortByStrategies measure the current implementation against.
func sortByComparator[T any, K cmp.Ordered](f Flow[T], key func(T) K) Flow[T] {
	return f.SortFunc(func(a, b T) int { return cmp.Compare(key(a), key(b)) })
}

func mul2(y int) int      { return y * 2 }
func add1(y int) int      { return y + 1 }
func mul3(y int) int      { return y * 3 }
func sub2(y int) int      { return y - 2 }
func div2(y int) int      { return y / 2 }
func add(a, b int) int    { return a + b }
func sum(f Flow[int]) int { return f.Reduce(add).OrElse(0) }
func even(n int) bool     { return n%2 == 0 }
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
