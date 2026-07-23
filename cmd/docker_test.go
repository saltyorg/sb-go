package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestRequestDockerJobUsesContextAndEncodesIgnoredContainers(t *testing.T) {
	var gotIgnore []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotIgnore = r.URL.Query()["ignore"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"job-1"}`))
	}))
	defer server.Close()

	job, err := requestDockerJob(
		context.Background(),
		server.URL+"/stop",
		[]string{"with space", "ampersand&name"},
		server.Client(),
	)
	if err != nil {
		t.Fatalf("request Docker job: %v", err)
	}
	if job.JobID != "job-1" {
		t.Fatalf("job ID = %q, want job-1", job.JobID)
	}
	if want := []string{"with space", "ampersand&name"}; !reflect.DeepEqual(gotIgnore, want) {
		t.Fatalf("ignore query = %#v, want %#v", gotIgnore, want)
	}
}

func TestRequestDockerJobRejectsMissingJobID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if _, err := requestDockerJob(context.Background(), server.URL, nil, server.Client()); err == nil {
		t.Fatal("expected missing job ID error")
	}
}

func TestWaitForJobCompletionHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitForJobCompletionWithClient(ctx, "job", server.URL, server.Client(), time.Hour, 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestShortContainerID(t *testing.T) {
	if got := shortContainerID("short"); got != "short" {
		t.Fatalf("short ID = %q", got)
	}
	if got := shortContainerID(""); got != "unknown" {
		t.Fatalf("empty ID = %q", got)
	}
	if got := shortContainerID("123456789012345"); got != "123456789012" {
		t.Fatalf("long ID = %q", got)
	}
}
