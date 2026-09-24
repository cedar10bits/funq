package funq

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type chainCase struct {
	name string
	in   string
	want int
}

// runChainCases calls fn once per case, so the same built Chain is exercised
// across every input.
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
				mustErr(t, err)
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

// bang and boom are the stage functions shared by the Track tests below that
// need a failing stage.
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
			mustErr(t, err)

			wantMsg := fmt.Sprintf(
				"funq: Groove pipeline failed at stage %d of %d: boom", tc.failAt, tc.stages,
			)
			assertEqual(t, wantMsg, err.Error())

			ge := mustGrooveError(t, err)
			assertEqual(t, tc.failAt, ge.Stage)
			assertEqual(t, tc.stages, ge.Of)
		})
	}
}

func TestGrooveError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        *GrooveError
		wantMsg    string
		wantUnwrap error
	}{
		{
			name:       "SingleStagePipeline",
			err:        &GrooveError{Stage: 1, Of: 1, Err: errors.New("underlying error")},
			wantMsg:    `funq: Groove pipeline failed at stage 1 of 1: underlying error`,
			wantUnwrap: errors.New("underlying error"),
		},
		{
			name:       "MidPipeline",
			err:        &GrooveError{Stage: 3, Of: 5, Err: errors.New("deep error")},
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

// TestGrooveErrorAsType covers errors.AsType[*GrooveError] on a real Play
// error: a single-stage failure and a nested Track, where each level keeps
// its own frame (see [GrooveError]).
func TestGrooveErrorAsType(t *testing.T) {
	t.Parallel()

	t.Run("SingleTrack", func(t *testing.T) {
		t.Parallel()
		track := buildTrack(3, 2)
		_, err := track.Play("a")
		mustErr(t, err)

		ge := mustGrooveError(t, err)
		assertEqual(t, 2, ge.Stage)
		assertEqual(t, 3, ge.Of)
	})

	t.Run("NestedTrack", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("inner boom")
		inner := Groove(strconv.Atoi).Jam(func(int) (int, error) { return 0, sentinel })
		outer := Groove(NilError(Identity[string])).
			Jam(inner.Play).
			Jam(NilError(strconv.Itoa))

		_, err := outer.Play("21")
		mustErr(t, err)

		outerGE := mustGrooveError(t, err)
		assertEqual(t, 2, outerGE.Stage)
		assertEqual(t, 3, outerGE.Of)

		innerGE := mustGrooveError(t, outerGE.Err)
		assertEqual(t, 2, innerGE.Stage)
		assertEqual(t, 2, innerGE.Of)
	})
}

// TestTrackErrorPropagation pins that errors.Is reaches through a Track's
// wrapping error to the underlying cause.
func TestTrackErrorPropagation(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("second function error")
	track := Groove(func(s string) (int, error) { return strconv.Atoi(s) }).
		Jam(func(int) (int, error) { return 0, sentinel })

	_, err := track.Play("42")
	mustErr(t, err)
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

	// inner.Play is already an Fe[string, int]; Jam takes it directly.
	outer := Groove(NilError(Identity[string])).
		Jam(inner.Play).
		Jam(NilError(strconv.Itoa))

	_, err := outer.Play("21")
	mustErr(t, err)
	if !errors.Is(err, sentinel) {
		t.Errorf("want errors.Is(%v, %v), got false", err, sentinel)
	}

	// inner is a 2-stage pipeline failing at its own stage 2; outer is a
	// 3-stage pipeline whose stage 2 (the call to inner.Play) is what failed.
	wantMsg := "funq: Groove pipeline failed at stage 2 of 3: " +
		"funq: Groove pipeline failed at stage 2 of 2: inner boom"
	assertEqual(t, wantMsg, err.Error())
}

// mustGrooveError returns the *GrooveError err carries, failing the test if
// it carries none.
func mustGrooveError(t *testing.T, err error) *GrooveError {
	t.Helper()
	ge, ok := errors.AsType[*GrooveError](err)
	if !ok {
		t.Fatalf("want errors.AsType[*GrooveError](%v) to succeed, got false", err)
	}
	return ge
}

// TestTrackOnBreak covers the compensation OnBreak registers: it runs only
// when a later stage fails or panics, using the value held at registration.
func TestTrackOnBreak(t *testing.T) {
	t.Parallel()

	t.Run("NotCalledOnSuccess", func(t *testing.T) {
		t.Parallel()
		calls := 0
		track := Groove(bang).
			OnBreak(func(string) error { calls++; return nil }).
			Jam(bang)

		got, err := track.Play("a")
		mustNoErr(t, err)
		assertEqual(t, "a!!", got)
		assertEqual(t, 0, calls)
	})

	t.Run("CalledOnLaterFailure", func(t *testing.T) {
		t.Parallel()
		var undone []string
		track := Groove(bang).
			OnBreak(func(s string) error { undone = append(undone, s); return nil }).
			Jam(boom)

		got, err := track.Play("a")
		assertEqual(t, "", got)
		assertEqual(t, []string{"a!"}, undone)
		assertEqual(t, "funq: Groove pipeline failed at stage 2 of 2: boom", err.Error())
		ge := mustGrooveError(t, err)
		assertEqual(t, 2, ge.Stage)
		assertEqual(t, 2, ge.Of)
	})

	t.Run("LIFOOrder", func(t *testing.T) {
		t.Parallel()
		var order []int
		record := func(n int) func(string) error {
			return func(string) error { order = append(order, n); return nil }
		}
		track := Groove(bang).
			OnBreak(record(1)).
			Jam(bang).
			OnBreak(record(2)).
			Jam(bang).
			OnBreak(record(3)).
			Jam(boom)

		_, err := track.Play("a")
		mustErr(t, err)
		assertEqual(t, []int{3, 2, 1}, order)
	})

	// The mirror of CalledOnLaterFailure: OnBreak registers once its value
	// exists, so one attached to the failing stage is never reached.
	t.Run("NotCalledWhenItsOwnStageFails", func(t *testing.T) {
		t.Parallel()
		calls := 0
		track := Groove(bang).
			Jam(boom).
			OnBreak(func(string) error { calls++; return nil })

		_, err := track.Play("a")
		mustErr(t, err)
		assertEqual(t, 0, calls)
	})

	t.Run("StageNumberingUnaffected", func(t *testing.T) {
		t.Parallel()
		noop := func(string) error { return nil }
		track := Groove(bang).
			OnBreak(noop).
			Jam(bang).
			OnBreak(noop).
			Jam(boom).
			OnBreak(noop)

		_, err := track.Play("a")
		mustErr(t, err)
		assertEqual(t, "funq: Groove pipeline failed at stage 3 of 3: boom", err.Error())
		ge := mustGrooveError(t, err)
		assertEqual(t, 3, ge.Stage)
		assertEqual(t, 3, ge.Of)
	})

	t.Run("CompensationErrorJoined", func(t *testing.T) {
		t.Parallel()
		track := Groove(bang).
			OnBreak(func(string) error { return errSentinel }).
			Jam(boom)

		got, err := track.Play("a")
		assertEqual(t, "", got)
		if !errors.Is(err, errSentinel) {
			t.Errorf("want errors.Is(%v, %v), got false", err, errSentinel)
		}
		ge := mustGrooveError(t, err)
		assertEqual(t, 2, ge.Stage)
		assertEqual(t, 2, ge.Of)
		want := "funq: Groove pipeline failed at stage 2 of 2: boom\n" +
			"funq: rollback for stage 1 failed: sentinel"
		assertEqual(t, want, err.Error())
	})

	// Two failing compensations join in rollback order. errors.Is still
	// reaches each of them through the joined error's multi-unwrap.
	t.Run("EveryCompensationErrorJoined", func(t *testing.T) {
		t.Parallel()
		errFirst := errors.New("first undo")
		errSecond := errors.New("second undo")
		track := Groove(bang).
			OnBreak(func(string) error { return errFirst }).
			Jam(bang).
			OnBreak(func(string) error { return errSecond }).
			Jam(boom)

		_, err := track.Play("a")
		for _, want := range []error{errFirst, errSecond} {
			if !errors.Is(err, want) {
				t.Errorf("want errors.Is(%v, %v), got false", err, want)
			}
		}
		want := "funq: Groove pipeline failed at stage 3 of 3: boom\n" +
			"funq: rollback for stage 2 failed: second undo\n" +
			"funq: rollback for stage 1 failed: first undo"
		assertEqual(t, want, err.Error())
	})

	// A panicking stage unwinds past Play's normal error handling, so rollback
	// runs from a defer — and the panic must reach the caller's recover unaltered.
	t.Run("PanicRunsCompensationsLIFO", func(t *testing.T) {
		t.Parallel()
		var order []int
		record := func(n int) func(string) error {
			return func(string) error { order = append(order, n); return nil }
		}
		track := Groove(bang).
			OnBreak(record(1)).
			Jam(bang).
			OnBreak(record(2)).
			Jam(NilError(func(string) string { panic(errSentinel) }))

		val, panicked := didPanic(func() { _, _ = track.Play("a") })
		isTrue(t, panicked, "want the stage panic to reach the caller")
		assertEqual[any](t, errSentinel, val)
		assertEqual(t, []int{2, 1}, order)
	})

	// PanicOnError is how a panic realistically reaches a track: its %w
	// wrapping must survive the rollback intact.
	t.Run("PanicOnErrorStageRollsBack", func(t *testing.T) {
		t.Parallel()
		calls := 0
		failing := func(string) (string, error) { return "", errSentinel }
		track := Groove(bang).
			OnBreak(func(string) error { calls++; return errors.New("discarded") }).
			Jam(NilError(PanicOnError(failing)))

		val, panicked := didPanic(func() { _, _ = track.Play("a") })
		isTrue(t, panicked, "want the stage panic to reach the caller")
		err, ok := val.(error)
		if !ok {
			t.Fatalf("panic value must be an error, got %#v", val)
		}
		if !errors.Is(err, errSentinel) {
			t.Errorf("want errors.Is(%v, %v), got false", err, errSentinel)
		}
		assertEqual(t, 1, calls)
	})

	// Preserving the stage's panic outranks reporting a compensation's own,
	// and containing each panic lets the remaining compensations still run.
	t.Run("CompensationPanicDoesNotMaskStagePanic", func(t *testing.T) {
		t.Parallel()
		var order []int
		track := Groove(bang).
			OnBreak(func(string) error {
				order = append(order, 1)
				return nil
			}).
			Jam(bang).
			OnBreak(func(string) error {
				order = append(order, 2)
				panic(errors.New("compensation panic"))
			}).
			Jam(NilError(func(string) string { panic(errSentinel) }))

		val, panicked := didPanic(func() { _, _ = track.Play("a") })
		isTrue(t, panicked, "want the stage panic to reach the caller")
		assertEqual[any](t, errSentinel, val)
		assertEqual(t, []int{2, 1}, order)
	})

	// A panic escaping Play here, instead of being joined into the returned
	// error, would fail this subtest outright.
	t.Run("CompensationPanicJoinedOnErrorPath", func(t *testing.T) {
		t.Parallel()
		errCompensation := errors.New("compensation panic")
		var order []int
		track := Groove(bang).
			OnBreak(func(string) error {
				order = append(order, 1)
				return nil
			}).
			Jam(bang).
			OnBreak(func(string) error {
				order = append(order, 2)
				panic(errCompensation)
			}).
			Jam(boom)

		_, err := track.Play("a")
		mustErr(t, err)
		assertEqual(t, []int{2, 1}, order)
		if !errors.Is(err, errCompensation) {
			t.Errorf("want errors.Is(%v, %v), got false", err, errCompensation)
		}
		want := "funq: Groove pipeline failed at stage 3 of 3: boom\n" +
			"funq: rollback for stage 2 panicked: compensation panic"
		assertEqual(t, want, err.Error())
	})

	// A non-error panic value has no identity to preserve, so it is folded
	// into the message instead.
	t.Run("NonErrorCompensationPanicJoined", func(t *testing.T) {
		t.Parallel()
		track := Groove(bang).
			OnBreak(func(string) error { panic("plain string") }).
			Jam(boom)

		_, err := track.Play("a")
		mustErr(t, err)
		want := "funq: Groove pipeline failed at stage 2 of 2: boom\n" +
			"funq: rollback for stage 1 panicked: plain string"
		assertEqual(t, want, err.Error())
	})

	// A stage can also leave with no recoverable panic value — runtime.Goexit,
	// or panic(nil) under GODEBUG=panicnil=1 — and rollback still runs for it.
	t.Run("AbnormalStageExitRunsCompensations", func(t *testing.T) {
		t.Parallel()
		var order []int
		record := func(n int) func(string) error {
			return func(string) error { order = append(order, n); return nil }
		}
		track := Groove(bang).
			OnBreak(record(1)).
			Jam(bang).
			OnBreak(record(2)).
			Jam(NilError(func(s string) string { runtime.Goexit(); return s }))

		var wg sync.WaitGroup
		wg.Go(func() {
			_, _ = track.Play("a")
		})
		wg.Wait()

		assertEqual(t, []int{2, 1}, order)
	})

	// A Track is a value: OnBreak registrations belong to the Play call that
	// runs them, not to the Track — branching and replaying must not leak them.
	t.Run("CompensationsDoNotLeakAcrossPlays", func(t *testing.T) {
		t.Parallel()
		var log []string
		record := func(name string) func(string) error {
			return func(string) error { log = append(log, name); return nil }
		}

		base := Groove(bang).OnBreak(record("base"))
		short := base.Jam(boom)
		long := base.Jam(bang).OnBreak(record("long")).Jam(boom)

		_, err := short.Play("a")
		mustErr(t, err)
		assertEqual(t, []string{"base"}, log)

		log = nil
		_, err = long.Play("a")
		mustErr(t, err)
		assertEqual(t, []string{"long", "base"}, log)

		// Replaying short sees only its own compensation, unaffected by the
		// one long registered on top of the prefix they share.
		log = nil
		_, err = short.Play("a")
		mustErr(t, err)
		assertEqual(t, []string{"base"}, log)
	})

	// Compensations are scoped to the Track that registered them — see
	// [Track.Play] — so inner and outer never trigger each other's rollback.
	t.Run("NestedTracksAreIndependent", func(t *testing.T) {
		t.Parallel()
		record := func(log *[]string, name string) func(string) error {
			return func(string) error { *log = append(*log, name); return nil }
		}

		t.Run("InnerFails", func(t *testing.T) {
			t.Parallel()
			var log []string
			inner := Groove(bang).OnBreak(record(&log, "inner")).Jam(boom)
			outer := Groove(bang).
				OnBreak(record(&log, "outer")).
				Jam(inner.Play).
				Jam(bang)

			_, err := outer.Play("a")
			mustErr(t, err)
			assertEqual(t, []string{"inner", "outer"}, log)
			want := "funq: Groove pipeline failed at stage 2 of 3: " +
				"funq: Groove pipeline failed at stage 2 of 2: boom"
			assertEqual(t, want, err.Error())
		})

		t.Run("OuterFailsAfterInnerSucceeds", func(t *testing.T) {
			t.Parallel()
			var log []string
			inner := Groove(bang).OnBreak(record(&log, "inner")).Jam(bang)
			outer := Groove(bang).
				OnBreak(record(&log, "outer")).
				Jam(inner.Play).
				Jam(boom)

			_, err := outer.Play("a")
			mustErr(t, err)
			assertEqual(t, []string{"outer"}, log)
			assertEqual(t, "funq: Groove pipeline failed at stage 3 of 3: boom", err.Error())
		})

		// Two stacked rollbacks: inner unwinds first, and the panic it lets
		// through reaches outer as an ordinary panicking stage.
		t.Run("InnerPanics", func(t *testing.T) {
			t.Parallel()
			var log []string
			inner := Groove(bang).
				OnBreak(record(&log, "inner")).
				Jam(NilError(func(string) string { panic(errSentinel) }))
			outer := Groove(bang).
				OnBreak(record(&log, "outer")).
				Jam(inner.Play).
				Jam(bang)

			val, panicked := didPanic(func() { _, _ = outer.Play("a") })
			isTrue(t, panicked, "want the inner stage panic to reach the caller")
			assertEqual[any](t, errSentinel, val)
			assertEqual(t, []string{"inner", "outer"}, log)
		})
	})
}

// TestChainBranchingFromSharedPrefix pins that extending a Chain returns a
// new value and leaves the chain it was built from untouched.
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
	mustErr(t, err)
	assertEqual(t, "funq: Groove pipeline failed at stage 2 of 2: boom", err.Error())

	_, err = long.Play("a")
	mustErr(t, err)
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
		ge := mustGrooveError(t, err)
		assertEqual(t, 1, ge.Stage)
		assertEqual(t, 2, ge.Of)
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

	// TestPanicError/MessagePrefix pins that PanicOnError's message matches
	// every other funq error/panic's "funq: " prefix.
	t.Run("MessagePrefix", func(t *testing.T) {
		t.Parallel()
		sentinel := errors.New("sentinel error")
		wrapped := PanicOnError(func(string) (int, error) { return 0, sentinel })

		func() {
			defer func() {
				err, ok := recover().(error)
				if !ok {
					t.Fatalf("panic value must be an error")
				}
				if !strings.HasPrefix(err.Error(), "funq: PanicOnError: ") {
					t.Errorf(`want error to start with "funq: PanicOnError: ", got %q`, err.Error())
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
