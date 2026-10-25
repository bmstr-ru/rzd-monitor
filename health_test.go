package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

type healthDatabase struct {
	err   error
	calls int
}

func (d *healthDatabase) Ping(context.Context) error { d.calls++; return d.err }

func TestHealthProbes(t *testing.T) {
	db := &healthDatabase{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := healthHandler(ctx, db)
	probe := func(path string, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != want {
			t.Fatalf("%s status %d, want %d", path, recorder.Code, want)
		}
	}
	probe("/livez", 200)
	if db.calls != 0 {
		t.Fatal("liveness must not depend on DB")
	}
	probe("/readyz", 200)
	db.err = errors.New("offline")
	probe("/livez", 200)
	probe("/readyz", 503)
	db.err = nil
	cancel()
	probe("/readyz", 503)
}
