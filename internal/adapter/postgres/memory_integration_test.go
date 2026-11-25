//go:build integration

package postgres

import (
	"errors"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/postexec"
)

const (
	memoryBound   = 64 << 20
	retainedBound = maxMessageBodyBytes + 1<<20
)

var memoryLimits = core.Limits{MaxRows: 100, MaxResultBytes: 32 << 10, MaxCellBytes: 4 << 10, Timeout: 15 * time.Second}

func peakHeapGrowth(fn func()) (peak, retained uint64) {
	defer debug.SetGCPercent(debug.SetGCPercent(10))
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc
	var top atomic.Uint64
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		var s runtime.MemStats
		for {
			runtime.ReadMemStats(&s)
			if s.HeapAlloc > top.Load() {
				top.Store(s.HeapAlloc)
			}
			select {
			case <-done:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	fn()
	close(done)
	<-stopped
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&ms)
	if ms.HeapAlloc > base {
		retained = ms.HeapAlloc - base
	}
	if top.Load() > base {
		peak = top.Load() - base
	}
	return peak, retained
}

func TestOversizedRowGivesFixedErrorInBoundedMemory(t *testing.T) {
	pg := startPostgres(t)
	res := productExecutor(t, pg)
	var err error
	peak, _ := peakHeapGrowth(func() {
		_, err = runQuery(t, res, "SELECT repeat('x', 20000000) AS v FROM generate_series(1, 20)", memoryLimits)
	})
	var ce *core.Error
	if !errors.As(err, &ce) || ce.Code != core.CodeUpstreamError || ce.Message != messageTooLarge {
		t.Fatalf("Execute = %v; want upstream_error %q", err, messageTooLarge)
	}
	t.Logf("peak heap growth = %d MiB", peak>>20)
	if peak > memoryBound {
		t.Fatalf("peak heap growth = %d MiB; want at most %d MiB", peak>>20, memoryBound>>20)
	}
	out, err := runQuery(t, res, "SELECT 1 AS n", memoryLimits)
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("next query = %+v, %v; want one row", out, err)
	}
}

func TestLargeCellsReadInBoundedMemory(t *testing.T) {
	pg := startPostgres(t)
	res := productExecutor(t, pg)
	var out core.Result
	var err error
	peak, retained := peakHeapGrowth(func() {
		out, err = runQuery(t, res, "SELECT repeat('x', 10000000) AS v FROM generate_series(1, 20)", memoryLimits)
		runtime.GC()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("peak heap growth = %d MiB, retained = %d KiB", peak>>20, retained>>10)
	if peak > memoryBound || retained > retainedBound {
		t.Fatalf("peak heap growth = %d MiB, retained = %d KiB; want at most %d MiB and %d KiB", peak>>20, retained>>10, memoryBound>>20, retainedBound>>10)
	}
	if !out.Truncated || out.TruncatedReason != core.TruncatedMaxResultBytes {
		t.Fatalf("read truncated %v %q; want max_result_bytes", out.Truncated, out.TruncatedReason)
	}
	long := strings.Repeat("x", 10000000)
	full := core.Result{Columns: out.Columns, Rows: make([][]any, 20)}
	for i := range full.Rows {
		full.Rows[i] = []any{long}
	}
	want := postexec.Apply(full, memoryLimits, nil)
	got := postexec.Apply(out, memoryLimits, nil)
	if !reflect.DeepEqual(got.Rows, want.Rows) || got.Truncated != want.Truncated || got.TruncatedReason != want.TruncatedReason || got.Bytes != want.Bytes {
		t.Fatalf("post-execute result = %d rows %v %q %d bytes; want %d rows %v %q %d bytes", len(got.Rows), got.Truncated, got.TruncatedReason, got.Bytes, len(want.Rows), want.Truncated, want.TruncatedReason, want.Bytes)
	}
}
