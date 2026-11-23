package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{Endpoint: srv.URL + "/", Token: "tok", PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewValidates(t *testing.T) {
	for _, cfg := range []Config{{Token: "x"}, {Endpoint: "http://h"}, {Endpoint: "not a url", Token: "x"}} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded, want error", cfg)
		}
	}
}

func TestAuthHeaderAndQuery(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Path != "/api/v1/vms/vm-1" || r.URL.Query().Get("confirm") != "web01" || r.URL.Query().Get("deleteDisks") != "true" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"task-1","status":"queued"}`))
	})
	task, err := c.DeleteVM(context.Background(), "vm-1", "web01", true)
	if err != nil || task == nil || task.ID != "task-1" {
		t.Fatalf("DeleteVM = %+v, %v", task, err)
	}
}

func TestDeleteVMNullTask(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("null\n"))
	})
	task, err := c.DeleteVM(context.Background(), "vm-1", "web01", false)
	if err != nil || task != nil {
		t.Fatalf("DeleteVM = %+v, %v; want nil task", task, err)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": `vm "x" not found`})
	})
	_, err := c.GetVM(context.Background(), "x")
	if !IsNotFound(err) || !strings.Contains(err.Error(), "not_found") {
		t.Fatalf("err = %v, want not found APIError", err)
	}
}

func TestWaitTask(t *testing.T) {
	polls := 0
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		polls++
		status := TaskRunning
		if polls >= 3 {
			status = TaskFailed
		}
		_ = json.NewEncoder(w).Encode(Task{ID: "t1", Type: "vm.start", Status: status, Error: "boom"})
	})
	_, err := c.WaitTask(context.Background(), "t1")
	if err == nil || !strings.Contains(err.Error(), "boom") || polls != 3 {
		t.Fatalf("WaitTask err = %v after %d polls", err, polls)
	}
}

func TestWaitTaskTimeout(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Task{ID: "t1", Status: TaskRunning})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.WaitTask(ctx, "t1"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
}

func TestGetBackupTargetNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"bt-1","name":"a"}]`))
	})
	if _, err := c.GetBackupTarget(context.Background(), "bt-2"); !IsNotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}
	if bt, err := c.GetBackupTarget(context.Background(), "bt-1"); err != nil || bt.Name != "a" {
		t.Fatalf("GetBackupTarget = %+v, %v", bt, err)
	}
}
