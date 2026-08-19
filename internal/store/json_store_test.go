package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"local-print-agent/internal/jobs"
)

func TestJSONStoreCRUDPersistsIndependentCopies(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.json")
	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	createdAt := time.Date(2026, time.August, 19, 1, 2, 3, 0, time.UTC)
	startedAt := createdAt.Add(time.Minute)
	expectedStartedAt := startedAt
	job := testJob("job-1", jobs.StatusQueued, createdAt)
	job.Payload = json.RawMessage(`{"ticket":"first"}`)
	job.StartedAt = &startedAt
	job.Error = &jobs.JobError{Code: "original", Message: "original error"}
	if err := store.Create(ctx, job); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	job.Payload[0] = '['
	job.Error.Message = "changed by caller"
	*job.StartedAt = createdAt.Add(2 * time.Hour)

	got, err := store.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got.Payload) != `{"ticket":"first"}` {
		t.Fatalf("Get().Payload = %s, want original payload", got.Payload)
	}
	if got.Error == nil || got.Error.Message != "original error" {
		t.Fatalf("Get().Error = %#v, want independent original error", got.Error)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(expectedStartedAt) {
		t.Fatalf("Get().StartedAt = %v, want %v", got.StartedAt, expectedStartedAt)
	}

	got.Payload[0] = '['
	got.Error.Message = "changed returned job"
	*got.StartedAt = createdAt.Add(3 * time.Hour)
	again, err := store.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if string(again.Payload) != `{"ticket":"first"}` || again.Error == nil || again.Error.Message != "original error" || again.StartedAt == nil || !again.StartedAt.Equal(expectedStartedAt) {
		t.Fatalf("second Get() returned a mutable internal value: %#v", again)
	}

	updated := *again
	updated.Status = jobs.StatusPrinting
	updated.Payload = json.RawMessage(`{"ticket":"updated"}`)
	updated.UpdatedAt = createdAt.Add(4 * time.Minute)
	if err := store.Update(ctx, &updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	updated.Payload[0] = '['
	updated.Status = jobs.StatusFailed

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "job-1" || listed[0].Status != jobs.StatusPrinting || string(listed[0].Payload) != `{"ticket":"updated"}` {
		t.Fatalf("List() = %#v, want persisted updated job", listed)
	}
	listed[0].Payload[0] = '['
	final, err := store.Get(ctx, "job-1")
	if err != nil {
		t.Fatalf("final Get() error = %v", err)
	}
	if string(final.Payload) != `{"ticket":"updated"}` {
		t.Fatalf("List() leaked a mutable internal pointer: %s", final.Payload)
	}

	if err := store.Create(ctx, testJob("job-1", jobs.StatusQueued, createdAt)); err == nil {
		t.Fatal("Create() duplicate ID error = nil, want error")
	}
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() missing error = %v, want ErrNotFound", err)
	}
	if err := store.Update(ctx, testJob("missing", jobs.StatusQueued, createdAt)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() missing error = %v, want ErrNotFound", err)
	}
}

func TestJSONStoreLoadsJobsAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.json")
	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	created := testJob("persists", jobs.StatusQueued, time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC))
	if err := store.Create(ctx, created); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	restarted, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() after restart error = %v", err)
	}
	got, err := restarted.Get(ctx, "persists")
	if err != nil {
		t.Fatalf("Get() after restart error = %v", err)
	}
	if got.ID != "persists" || string(got.Payload) != `{"ticket":"persists"}` {
		t.Fatalf("Get() after restart = %#v", got)
	}
}

func TestJSONStoreRejectsCorruptExistingFileWithoutOverwritingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	corrupt := []byte(`{"jobs":[`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := NewJSONStore(path); err == nil {
		t.Fatal("NewJSONStore() corrupt JSON error = nil, want diagnostic error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt file changed: got %q, want %q", got, corrupt)
	}
}

func TestJSONStoreRecoversOnlyInterruptedJobs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.json")
	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	base := time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC)
	for _, job := range []*jobs.Job{
		testJob("rendering", jobs.StatusRendering, base),
		testJob("printing", jobs.StatusPrinting, base.Add(time.Second)),
		testJob("queued", jobs.StatusQueued, base.Add(2*time.Second)),
		testJob("succeeded", jobs.StatusSucceeded, base.Add(3*time.Second)),
		testJob("failed", jobs.StatusFailed, base.Add(4*time.Second)),
	} {
		if err := store.Create(ctx, job); err != nil {
			t.Fatalf("Create(%q) error = %v", job.ID, err)
		}
	}

	if err := store.RecoverInterrupted(ctx); err != nil {
		t.Fatalf("RecoverInterrupted() error = %v", err)
	}
	for _, id := range []string{"rendering", "printing"} {
		job, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", id, err)
		}
		if job.Status != jobs.StatusFailed {
			t.Errorf("Get(%q).Status = %q, want failed", id, job.Status)
		}
		if job.Error == nil || job.Error.Code != jobs.ErrorCode("SERVICE_RESTARTED") || job.Error.Message == "" {
			t.Errorf("Get(%q).Error = %#v, want readable SERVICE_RESTARTED error", id, job.Error)
		}
		if job.FinishedAt == nil || job.UpdatedAt.IsZero() {
			t.Errorf("Get(%q) timestamps = finished %v updated %v, want both set", id, job.FinishedAt, job.UpdatedAt)
		}
	}
	for _, id := range []string{"queued", "succeeded", "failed"} {
		job, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", id, err)
		}
		if job.Status != jobs.Status(id) {
			t.Errorf("Get(%q).Status = %q, want unchanged %q", id, job.Status, id)
		}
	}
}

func TestJSONStoreListHasDeterministicCreatedAtThenIDOrder(t *testing.T) {
	ctx := context.Background()
	store, err := NewJSONStore(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	createdAt := time.Date(2026, 8, 19, 3, 0, 0, 0, time.UTC)
	for _, job := range []*jobs.Job{
		testJob("z", jobs.StatusQueued, createdAt.Add(time.Minute)),
		testJob("b", jobs.StatusQueued, createdAt),
		testJob("a", jobs.StatusQueued, createdAt),
	} {
		if err := store.Create(ctx, job); err != nil {
			t.Fatalf("Create(%q) error = %v", job.ID, err)
		}
	}
	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	if want := []string{"a", "b", "z"}; !equalStrings(ids, want) {
		t.Fatalf("List() IDs = %v, want %v", ids, want)
	}
}

func TestJSONStoreHonorsCanceledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	store, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Create(ctx, testJob("cancelled", jobs.StatusQueued, time.Now())); !errors.Is(err, context.Canceled) {
		t.Errorf("Create() error = %v, want context.Canceled", err)
	}
	if err := store.Update(ctx, testJob("cancelled", jobs.StatusQueued, time.Now())); !errors.Is(err, context.Canceled) {
		t.Errorf("Update() error = %v, want context.Canceled", err)
	}
	if _, err := store.Get(ctx, "cancelled"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get() error = %v, want context.Canceled", err)
	}
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List() error = %v, want context.Canceled", err)
	}
	if err := store.RecoverInterrupted(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("RecoverInterrupted() error = %v, want context.Canceled", err)
	}
}

func TestJSONStoreConcurrentCreatesAndSuccessfulWritesLeaveNoTemporaryFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(dir, "jobs.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	const count = 24
	var group sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			errs <- store.Create(ctx, testJob(string(rune('a'+i)), jobs.StatusQueued, time.Unix(int64(i), 0).UTC()))
		}(i)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create() error = %v", err)
		}
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != count {
		t.Fatalf("List() length = %d, want %d", len(listed), count)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "jobs.json" {
			t.Errorf("temporary file left behind after successful writes: %q", entry.Name())
		}
	}
}

func TestJSONStoreRemovesTemporaryFileWhenReplacementFails(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(dir, "jobs.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	replaceErr := errors.New("replacement blocked")
	store.rename = func(_, _ string) error { return replaceErr }

	err = store.Create(ctx, testJob("unsaved", jobs.StatusQueued, time.Now()))
	if !errors.Is(err, replaceErr) {
		t.Fatalf("Create() error = %v, want replacement error", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed replacement left temporary files: %v", entries)
	}
	if _, err := store.Get(ctx, "unsaved"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed Create() changed in-memory state: %v", err)
	}
}

func testJob(id string, status jobs.Status, createdAt time.Time) *jobs.Job {
	return &jobs.Job{
		ID:        id,
		Type:      jobs.JobTypeBalloon,
		Payload:   json.RawMessage(`{"ticket":"` + id + `"}`),
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
