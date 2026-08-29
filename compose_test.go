package funq

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
)

type chainCase struct {
	name string
	in   string
	want int
}

// runChainCases calls fn once per case, so the same built Chain/Track is
// exercised across every input.
func runChainCases(t *testing.T, fn func(string) int, tests []chainCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqual(t, tc.want, fn(tc.in))
		})
	}
}

// TestChainLengths covers chains of several lengths, including one well
// past five stages, to pin that Chain has no fixed arity.
func TestChainLengths(t *testing.T) {
	t.Parallel()
	atoi := PanicOnError(strconv.Atoi)

	t.Run("OneStage", func(t *testing.T) {
		t.Parallel()
		runChainCases(t, Compose(atoi).Run, []chainCase{
			{name: "want0", in: "0", want: 0},
			{name: "want50", in: "50", want: 50},
		})
	})

	t.Run("TwoStages", func(t *testing.T) {
		t.Parallel()
		chain := Compose(atoi).Then(mul2)
		runChainCases(t, chain.Run, []chainCase{
			{name: "want0", in: "0", want: 0},
			{name: "want100", in: "50", want: 100},
		})
	})

	t.Run("ThreeStages", func(t *testing.T) {
		t.Parallel()
		atoiOrZero := IgnoreError(strconv.Atoi, 0)
		chain := Compose(atoiOrZero).Then(mul2).Then(add1)
		runChainCases(t, chain.Run, []chainCase{
			{name: "want1", in: "0", want: 1},
			{name: "want101", in: "50", want: 101},
		})
	})

	t.Run("FourStages", func(t *testing.T) {
		t.Parallel()
		chain := Compose(atoi).Then(mul2).Then(add1).Then(mul3)
		runChainCases(t, chain.Run, []chainCase{
			{name: "want9", in: "1", want: 9},
		})
	})

	t.Run("FiveStages", func(t *testing.T) {
		t.Parallel()
		chain := Compose(atoi).Then(mul2).Then(add1).Then(mul3).Then(sub2)
		runChainCases(t, chain.Run, []chainCase{
			{name: "want7", in: "1", want: 7},
			{name: "want13", in: "2", want: 13},
		})
	})

	t.Run("EightStages", func(t *testing.T) {
		t.Parallel()
		chain := Compose(atoi).Then(mul2).Then(add1).Then(mul3).Then(sub2).
			Then(div2).Then(add1).Then(mul2)
		runChainCases(t, chain.Run, []chainCase{
			{name: "want8", in: "1", want: 8},
			{name: "want14", in: "2", want: 14},
		})
	})
}

type trackCase struct {
	name    string
	in      string
	want    int
	wantErr bool
}

func runTrackCases(t *testing.T, fn func(string) (int, error), tests []trackCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := fn(tc.in)
			assertEqual(t, tc.want, got)
			if tc.wantErr {
				if err == nil {
					t.Errorf("want an error, got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestTrackLengths covers tracks of several lengths, the same way
// TestChainLengths does for Chain.
func TestTrackLengths(t *testing.T) {
	t.Parallel()

	t.Run("OneStage", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi)
		runTrackCases(t, track.Play, []trackCase{
			{name: "want0", in: "0", want: 0},
			{name: "want50", in: "50", want: 50},
			{name: "wantError", in: "a", want: 0, wantErr: true},
		})
	})

	t.Run("TwoStages", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi).Jam(NilError(Compose(mul2).Then(add1).Run))
		runTrackCases(t, track.Play, []trackCase{
			{name: "want1", in: "0", want: 1},
			{name: "want101", in: "50", want: 101},
			{name: "wantError", in: "a", want: 0, wantErr: true},
		})
	})

	t.Run("ThreeStages", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi).Jam(NilError(mul2)).Jam(NilError(add1))
		runTrackCases(t, track.Play, []trackCase{
			{name: "want1", in: "0", want: 1},
			{name: "want101", in: "50", want: 101},
			{name: "wantError", in: "a", want: 0, wantErr: true},
		})
	})

	t.Run("FourStages", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi).Jam(NilError(mul2)).Jam(NilError(add1)).Jam(NilError(mul3))
		runTrackCases(t, track.Play, []trackCase{
			{name: "want9", in: "1", want: 9, wantErr: false},
			{name: "wantError", in: "a", want: 0, wantErr: true},
		})
	})

	t.Run("FiveStages", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi).
			Jam(NilError(mul2)).Jam(NilError(add1)).Jam(NilError(mul3)).Jam(NilError(sub2))
		runTrackCases(t, track.Play, []trackCase{
			{name: "want7", in: "1", want: 7},
			{name: "want13", in: "2", want: 13},
			{name: "wantError", in: "a", want: 0, wantErr: true},
		})
	})

	t.Run("EightStages", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi).
			Jam(NilError(mul2)).Jam(NilError(add1)).Jam(NilError(mul3)).Jam(NilError(sub2)).
			Jam(NilError(div2)).Jam(NilError(add1)).Jam(NilError(mul2))
		runTrackCases(t, track.Play, []trackCase{
			{name: "want8", in: "1", want: 8},
			{name: "want14", in: "2", want: 14},
			{name: "wantError", in: "a", want: 0, wantErr: true},
		})
	})
}

// bang and boom are the stage functions shared by the Track failure-position
// tests below.
func bang(s string) (string, error) { return s + "!", nil }

func boom(string) (string, error) { return "", errors.New("boom") }

// buildTrack returns an n-stage Track[string, string] where the stage at
// position failAt (1-based) is boom and every other stage is bang.
func buildTrack(n, failAt int) Track[string, string] {
	stageAt := func(i int) Fe[string, string] {
		if i == failAt {
			return boom
		}
		return bang
	}
	track := Groove(stageAt(1))
	for i := 2; i <= n; i++ {
		track = track.Jam(stageAt(i))
	}
	return track
}

// TestTrackFailurePosition pins that a failure reports its own position:
// which stage failed, and how many stages the track has. Short tracks (up
// to three stages) cover every position. The five- and eight-stage tracks
// sample only the first, middle, and last position.
func TestTrackFailurePosition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		stages, failAt int
	}{
		{1, 1},
		{2, 1},
		{2, 2},
		{3, 1},
		{3, 2},
		{3, 3},
		{5, 1},
		{5, 3},
		{5, 5},
		{8, 1},
		{8, 4},
		{8, 8},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%dStages/fail@%d", tc.stages, tc.failAt), func(t *testing.T) {
			t.Parallel()
			track := buildTrack(tc.stages, tc.failAt)
			_, err := track.Play("a")
			if err == nil {
				t.Fatalf("want an error, got nil")
			}

			wantMsg := fmt.Sprintf(
				"funq: Groove pipeline failed at stage %d of %d: boom", tc.failAt, tc.stages,
			)
			assertEqual(t, wantMsg, err.Error())

			var ge *grooveError
			if !errors.As(err, &ge) {
				t.Fatalf("want errors.As(%v, %T) to succeed, got false", err, ge)
			}
			assertEqual(t, tc.failAt, ge.stage)
			assertEqual(t, tc.stages, ge.of)
		})
	}
}

func TestGrooveError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        *grooveError
		wantMsg    string
		wantUnwrap error
	}{
		{
			name:       "SingleStagePipeline",
			err:        &grooveError{stage: 1, of: 1, err: errors.New("underlying error")},
			wantMsg:    `funq: Groove pipeline failed at stage 1 of 1: underlying error`,
			wantUnwrap: errors.New("underlying error"),
		},
		{
			name:       "MidPipeline",
			err:        &grooveError{stage: 3, of: 5, err: errors.New("deep error")},
			wantMsg:    `funq: Groove pipeline failed at stage 3 of 5: deep error`,
			wantUnwrap: errors.New("deep error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqual(t, tc.wantMsg, tc.err.Error())
			assertEqual(t, tc.wantUnwrap, tc.err.Unwrap())
		})
	}
}

// TestTrackErrorPropagation pins that errors.Is reaches through a Track's
// wrapping error to the underlying cause.
func TestTrackErrorPropagation(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("second function error")
	track := Groove(func(s string) (int, error) { return strconv.Atoi(s) }).
		Jam(func(int) (int, error) { return 0, sentinel })

	_, err := track.Play("42")
	if err == nil {
		t.Fatalf("want an error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("want errors.Is(%v, %v), got false", err, sentinel)
	}
	assertEqual(
		t,
		`funq: Groove pipeline failed at stage 2 of 2: second function error`,
		err.Error(),
	)
}

// TestTrackNestedPipeline covers a stage that runs another, independently
// built Track (inner.Play) rather than a plain function, exercising
// Track.Play's nested-stage-numbering contract. errors.Is still reaches
// the root cause through both layers.
func TestTrackNestedPipeline(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("inner boom")
	inner := Groove(strconv.Atoi).Jam(func(int) (int, error) { return 0, sentinel })

	// inner.Play is already an Fe[string, int]; Jam takes it directly, the
	// same interop TestMethodValueInterop pins.
	outer := Groove(NilError(Identity[string])).
		Jam(inner.Play).
		Jam(NilError(strconv.Itoa))

	_, err := outer.Play("21")
	if err == nil {
		t.Fatalf("want an error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("want errors.Is(%v, %v), got false", err, sentinel)
	}

	// inner is a 2-stage pipeline failing at its own stage 2; outer is a
	// 3-stage pipeline whose stage 2 (the call to inner.Play) is what failed.
	wantMsg := "funq: Groove pipeline failed at stage 2 of 3: " +
		"funq: Groove pipeline failed at stage 2 of 2: inner boom"
	assertEqual(t, wantMsg, err.Error())
}

// TestChainBranchingFromSharedPrefix pins that extending a Chain returns a
// new value and leaves the chain it was built from untouched: base and long
// share the same first two stages, but appending to long must not change
// what base itself computes.
func TestChainBranchingFromSharedPrefix(t *testing.T) {
	t.Parallel()
	base := Compose(mul2).Then(add1)
	short := base
	long := base.Then(mul3).Then(sub2)

	assertEqual(t, 5, short.Run(2))
	assertEqual(t, 13, long.Run(2))
}

// TestTrackBranchingFromSharedPrefix pins the same independence for Track,
// where it matters more: a Track's failing-stage position is captured
// per-Jam at build time while its total stage count is threaded through at
// Play time, so branching from a shared prefix is the case that would catch
// the two getting out of sync.
func TestTrackBranchingFromSharedPrefix(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	ok := func(s string) (string, error) { return s + "!", nil }
	fail := func(string) (string, error) { return "", sentinel }

	base := Groove(ok).Jam(fail)
	short := base
	long := base.Jam(ok).Jam(ok)

	_, err := short.Play("a")
	if err == nil {
		t.Fatalf("want an error, got nil")
	}
	assertEqual(t, "funq: Groove pipeline failed at stage 2 of 2: boom", err.Error())

	_, err = long.Play("a")
	if err == nil {
		t.Fatalf("want an error, got nil")
	}
	assertEqual(t, "funq: Groove pipeline failed at stage 2 of 4: boom", err.Error())
}

// TestMethodValueInterop pins that Chain.Run and Track.Play are themselves
// method values assignable to Fp/Fe.
func TestMethodValueInterop(t *testing.T) {
	t.Parallel()

	t.Run("ChainRunAsFp", func(t *testing.T) {
		t.Parallel()
		chain := Compose(PanicOnError(strconv.Atoi)).Then(mul2)
		var g Fp[string, int] = chain.Run
		assertEqual(t, 100, g("50"))
	})

	t.Run("TrackPlayAsFe", func(t *testing.T) {
		t.Parallel()
		track := Groove(strconv.Atoi).Jam(NilError(mul2))
		var f Fe[string, int] = track.Play
		got, err := f("50")
		mustNoErr(t, err)
		assertEqual(t, 100, got)
	})
}

func TestIgnoreError(t *testing.T) {
	t.Parallel()

	t.Run("SuccessCase", func(t *testing.T) {
		t.Parallel()
		successFunc := func(s string) (int, error) { return strconv.Atoi(s) }
		wrapped := IgnoreError(successFunc, -1)

		result := wrapped("42")
		assertEqual(t, 42, result)

		result = wrapped("0")
		assertEqual(t, 0, result)
	})

	t.Run("ErrorCase", func(t *testing.T) {
		t.Parallel()
		errorFunc := func(s string) (int, error) { return 0, errors.New("test error") }
		wrapped := IgnoreError(errorFunc, -1)

		result := wrapped("anything")
		assertEqual(t, -1, result)
	})

	t.Run("StringType", func(t *testing.T) {
		t.Parallel()
		successFunc := func(n int) (string, error) { return strconv.Itoa(n), nil }
		wrapped := IgnoreError(successFunc, "default")

		result := wrapped(42)
		assertEqual(t, "42", result)

		errorFunc := func(n int) (string, error) { return "", errors.New("conversion error") }
		wrappedError := IgnoreError(errorFunc, "default")

		result = wrappedError(42)
		assertEqual(t, "default", result)
	})

	t.Run("ComplexType", func(t *testing.T) {
		t.Parallel()
		type Person struct {
			Name string
			Age  int
		}

		successFunc := func(id int) (Person, error) {
			if id == 1 {
				return Person{Name: "John", Age: 30}, nil
			}
			return Person{}, errors.New("not found")
		}
		defaultPerson := Person{Name: "Unknown", Age: 0}
		wrapped := IgnoreError(successFunc, defaultPerson)

		result := wrapped(1)
		assertEqual(t, Person{Name: "John", Age: 30}, result)

		result = wrapped(999)
		assertEqual(t, defaultPerson, result)
	})

	t.Run("ZeroValueAsDefault", func(t *testing.T) {
		t.Parallel()
		errorFunc := func(s string) (int, error) { return 0, errors.New("always fails") }
		wrapped := IgnoreError(errorFunc, 0)

		result := wrapped("anything")
		assertEqual(t, 0, result)
	})

	t.Run("IntegrationWithTrack", func(t *testing.T) {
		t.Parallel()
		parseAndDouble := Groove(strconv.Atoi).Jam(func(n int) (int, error) { return n * 2, nil })

		wrapped := IgnoreError(parseAndDouble.Play, -999)

		result := wrapped("21")
		assertEqual(t, 42, result)

		result = wrapped("invalid")
		assertEqual(t, -999, result)
	})
}

func TestErrOnNone(t *testing.T) {
	t.Parallel()

	lookup := func(name string) Optional[int] {
		return From("ada", "alan").Find(Equal(name)).Map(func(s string) int { return len(s) })
	}

	t.Run("Present", func(t *testing.T) {
		t.Parallel()
		n, err := ErrOnNone(lookup, errSentinel)("alan")
		assertEqual(t, 4, n)
		mustNoErr(t, err)
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		n, err := ErrOnNone(lookup, errSentinel)("grace")
		assertEqual(t, 0, n)
		if !errors.Is(err, errSentinel) {
			t.Errorf("want errors.Is(%v, %v), got false", err, errSentinel)
		}
	})

	t.Run("AsTrackStage", func(t *testing.T) {
		t.Parallel()
		pipeline := Groove(ErrOnNone(lookup, errSentinel)).
			Jam(func(n int) (string, error) { return strconv.Itoa(n), nil })

		got, err := pipeline.Play("ada")
		mustNoErr(t, err)
		assertEqual(t, "3", got)

		// A failure inside the adapter still plays like any other stage's
		// failure: Play stops there and reports which stage failed.
		_, err = pipeline.Play("grace")
		if !errors.Is(err, errSentinel) {
			t.Errorf("want errors.Is(%v, %v), got false", err, errSentinel)
		}
		var ge *grooveError
		if !errors.As(err, &ge) {
			t.Fatalf("want errors.As(%v, %T) to succeed, got false", err, ge)
		}
		assertEqual(t, 1, ge.stage)
		assertEqual(t, 2, ge.of)
	})
}

func TestPanicError(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		successFunc := func(s string) (int, error) { return strconv.Atoi(s) }
		wrapped := PanicOnError(successFunc)

		result := wrapped("42")
		assertEqual(t, 42, result)

		result = wrapped("0")
		assertEqual(t, 0, result)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		errorFunc := func(s string) (int, error) { return 0, errors.New("test error") }
		wrapped := PanicOnError(errorFunc)

		if _, panicked := didPanic(func() {
			wrapped("anything")
		}); !panicked {
			t.Errorf("want fn to panic, it did not")
		}
	})

	t.Run("RecoverPreservesOriginalError", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("sentinel error")
		errorFunc := func(s string) (int, error) { return 0, sentinel }
		wrapped := PanicOnError(errorFunc)

		func() {
			defer func() {
				r := recover()
				err, ok := r.(error)
				if !ok {
					t.Fatalf("panic value must be an error, got %#v", r)
				}
				if !errors.Is(err, sentinel) {
					t.Errorf("want errors.Is(%v, %v), got false", err, sentinel)
				}
			}()
			wrapped("anything")
		}()
	})
}

func TestIdentity(t *testing.T) {
	t.Parallel()
	for v := range 3 {
		assertEqual(t, v, Identity(v))
	}
}

func TestConst(t *testing.T) {
	t.Parallel()
	fn := Const[int](1)
	for v := range 3 {
		assertEqual(t, 1, fn(v))
	}

	t.Run("ChangesType", func(t *testing.T) {
		t.Parallel()
		got := From(1, 2, 3).Map(Const[int]("x")).Slice()
		want := []string{"x", "x", "x"}
		assertEqual(t, want, got)
	})
}

func BenchmarkChain(b *testing.B) {
	parse := PanicOnError(strconv.Atoi)

	traditionalChain := func(s string) int {
		n := parse(s)
		n = mul2(n)
		n = add1(n)
		n = mul3(n)
		return n
	}
	b.Run("NoChain", func(b *testing.B) {
		for b.Loop() {
			_ = traditionalChain("42")
		}
	})

	chain := Compose(parse).Then(mul2).Then(add1).Then(mul3)
	b.Run("Chain", func(b *testing.B) {
		for b.Loop() {
			_ = chain.Run("42")
		}
	})
}

func BenchmarkTrack(b *testing.B) {
	parse := strconv.Atoi
	twice := NilError(mul2)
	addOne := NilError(add1)
	multiplyByThree := NilError(mul3)

	traditionalChain := func(s string) (int, error) {
		n, err := parse(s)
		if err != nil {
			return 0, err
		}
		n, err = twice(n)
		if err != nil {
			return 0, err
		}
		n, err = addOne(n)
		if err != nil {
			return 0, err
		}
		n, err = multiplyByThree(n)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	b.Run("NoTrack", func(b *testing.B) {
		for b.Loop() {
			_, _ = traditionalChain("42")
		}
	})

	track := Groove(parse).Jam(twice).Jam(addOne).Jam(multiplyByThree)
	b.Run("Track", func(b *testing.B) {
		for b.Loop() {
			_, _ = track.Play("42")
		}
	})
}

func BenchmarkChainHeavy(b *testing.B) {
	heavyCompute := func(base int) int {
		sum := 0
		for i := range 10000 {
			sum += base + i
		}
		return sum
	}

	parse := PanicOnError(strconv.Atoi)
	twice := func(n int) int { return heavyCompute(n) * 2 }
	addOne := func(n int) int { return heavyCompute(n) + 1 }
	multiplyByThree := func(n int) int { return heavyCompute(n) * 3 }

	noChain := func(s string) int {
		n := parse(s)
		n = twice(n)
		n = addOne(n)
		n = multiplyByThree(n)
		return n
	}
	b.Run("NoChain", func(b *testing.B) {
		for b.Loop() {
			_ = noChain("42")
		}
	})

	chain := Compose(parse).Then(twice).Then(addOne).Then(multiplyByThree)
	b.Run("Chain", func(b *testing.B) {
		for b.Loop() {
			_ = chain.Run("42")
		}
	})
}

func BenchmarkTrackHeavy(b *testing.B) {
	heavyCompute := func(base int) int {
		sum := 0
		for i := range 10000 {
			sum += base + i
		}
		return sum
	}

	parse := strconv.Atoi
	twice := NilError(Compose(heavyCompute).Then(mul2).Run)
	addOne := NilError(Compose(heavyCompute).Then(add1).Run)
	multiplyByThree := NilError(Compose(heavyCompute).Then(mul3).Run)

	noTrack := func(s string) (int, error) {
		n, err := parse(s)
		if err != nil {
			return 0, err
		}
		n, err = twice(n)
		if err != nil {
			return 0, err
		}
		n, err = addOne(n)
		if err != nil {
			return 0, err
		}
		n, err = multiplyByThree(n)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	b.Run("NoTrack", func(b *testing.B) {
		for b.Loop() {
			_, _ = noTrack("42")
		}
	})

	track := Groove(parse).Jam(twice).Jam(addOne).Jam(multiplyByThree)
	b.Run("Track", func(b *testing.B) {
		for b.Loop() {
			_, _ = track.Play("42")
		}
	})
}
