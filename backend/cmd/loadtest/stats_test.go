package main

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	values := []time.Duration{4 * time.Millisecond, time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}
	result := summarize(values)
	if result.Count != 4 || result.P50 != 2 || result.P95 != 4 || result.P99 != 4 || result.Max != 4 || values[0] != 4*time.Millisecond {
		t.Fatalf("invalid percentiles: %+v", result)
	}
	if summarize(nil).Count != 0 {
		t.Fatal("empty sample has a count")
	}
}

func TestParseRates(t *testing.T) {
	rates, err := parseRates("1,5,10")
	if err != nil || !reflect.DeepEqual(rates, []int{1, 5, 10}) {
		t.Fatal(rates, err)
	}
	for _, invalid := range []string{"", "0", "-1", "1001", "1,no"} {
		if _, err := parseRates(invalid); err == nil {
			t.Fatal("accepted", invalid)
		}
	}
}

func TestFailuresRetainSyncValidationErrors(t *testing.T) {
	stats := &statistics{}
	stats.failure("sync", errors.New("invalid sync cursor"))
	stats.failure("protocol", errors.New("invalid frame"))
	stats.failure("http", errors.New("connection refused"))
	if stats.syncErrors != 1 || stats.protocolErrors != 1 || stats.httpErrors != 1 || len(stats.errorSamples) != 3 {
		t.Fatal("load validation must retain every failure category")
	}
}
