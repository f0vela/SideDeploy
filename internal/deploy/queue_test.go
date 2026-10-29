package deploy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSession struct {
	mu       sync.Mutex
	uploads  []string
	deletes  []string
	mkdirs   map[string]int
	attempts map[string]int
	failOnce map[string]bool
	openErr  error

	live    int32
	maxLive int32
	closed  int32
}

func newFake() *fakeSession {
	return &fakeSession{mkdirs: map[string]int{}, attempts: map[string]int{}, failOnce: map[string]bool{}}
}

func (f *fakeSession) Upload(ctx context.Context, local, remote string) error {
	live := atomic.AddInt32(&f.live, 1)
	defer atomic.AddInt32(&f.live, -1)
	for {
		max := atomic.LoadInt32(&f.maxLive)
		if live <= max || atomic.CompareAndSwapInt32(&f.maxLive, max, live) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts[remote]++
	if f.failOnce[remote] {
		delete(f.failOnce, remote)
		return errors.New("fallo pasajero")
	}
	f.uploads = append(f.uploads, remote)
	return nil
}

func (f *fakeSession) MkdirAll(ctx context.Context, dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mkdirs[dir]++
	return nil
}

func (f *fakeSession) Delete(ctx context.Context, remote string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, remote)
	return nil
}

func (f *fakeSession) Close() error {
	atomic.AddInt32(&f.closed, 1)
	return nil
}

func fakeJobs(rels ...string) []Job {
	jobs := make([]Job, len(rels))
	for i, rel := range rels {
		jobs[i] = Job{Rel: rel, Local: "/local/" + rel, Remote: "/remote/" + rel}
	}
	return jobs
}

func TestRunUploadsAndCachesDirs(t *testing.T) {
	f := newFake()
	var got []Result
	res := Run(context.Background(), func(context.Context) (Session, error) { return f, nil },
		fakeJobs("a/one.php", "a/two.php", "b/three.php"), 1, func(r Result) { got = append(got, r) })

	if len(res) != 3 || len(got) != 3 {
		t.Fatalf("results = %d, reported = %d", len(res), len(got))
	}
	for _, r := range res {
		if r.Err != nil {
			t.Errorf("%s: %v", r.Rel, r.Err)
		}
	}
	if len(f.uploads) != 3 {
		t.Fatalf("uploads = %v", f.uploads)
	}
	if f.mkdirs["/remote/a"] != 1 || f.mkdirs["/remote/b"] != 1 {
		t.Fatalf("mkdirs = %v, want one per folder", f.mkdirs)
	}
	if f.closed != 1 {
		t.Fatalf("close calls = %d", f.closed)
	}
}

func TestRunDeletesWithoutMkdir(t *testing.T) {
	f := newFake()
	res := Run(context.Background(), func(context.Context) (Session, error) { return f, nil },
		[]Job{{Rel: "old.php", Remote: "/remote/old.php", Delete: true}}, 1, func(Result) {})
	if len(res) != 1 || res[0].Err != nil {
		t.Fatalf("results = %+v", res)
	}
	if len(f.deletes) != 1 || len(f.mkdirs) != 0 {
		t.Fatalf("deletes = %v mkdirs = %v", f.deletes, f.mkdirs)
	}
}

func TestRunRetriesOnce(t *testing.T) {
	f := newFake()
	f.failOnce["/remote/a.php"] = true
	res := Run(context.Background(), func(context.Context) (Session, error) { return f, nil },
		fakeJobs("a.php"), 1, func(Result) {})
	if len(res) != 1 || res[0].Err != nil {
		t.Fatalf("results = %+v", res)
	}
	if f.attempts["/remote/a.php"] != 2 {
		t.Fatalf("attempts = %d", f.attempts["/remote/a.php"])
	}
}

func TestRunDialErrorReachesEveryJob(t *testing.T) {
	f := newFake()
	f.openErr = errors.New("sin conexión")
	res := Run(context.Background(), func(context.Context) (Session, error) { return nil, f.openErr },
		fakeJobs("a.php", "b.php", "c.php"), 2, func(Result) {})
	if len(res) != 3 {
		t.Fatalf("results = %d", len(res))
	}
	for _, r := range res {
		if r.Err == nil {
			t.Errorf("%s has no error", r.Rel)
		}
	}
}

func TestRunConcurrency(t *testing.T) {
	f := newFake()
	jobs := fakeJobs("a.php", "b.php", "c.php", "d.php", "e.php", "f.php")
	Run(context.Background(), func(context.Context) (Session, error) { return f, nil }, jobs, 3, func(Result) {})
	if f.maxLive > 3 {
		t.Fatalf("in flight = %d, want at most 3", f.maxLive)
	}
	if f.maxLive < 2 {
		t.Fatalf("in flight = %d, want the workers to overlap", f.maxLive)
	}
}

func TestRunCancel(t *testing.T) {
	f := newFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	res := Run(ctx, func(context.Context) (Session, error) { return f, nil },
		fakeJobs("a.php", "b.php"), 2, func(Result) {})
	if time.Since(start) > time.Second {
		t.Fatal("Run did not return soon after cancel")
	}
	for _, r := range res {
		if r.Err == nil {
			t.Errorf("%s uploaded after cancel", r.Rel)
		}
	}
}

func TestRunZeroWorkersAndJobs(t *testing.T) {
	if res := Run(context.Background(), nil, nil, 5, func(Result) {}); res != nil {
		t.Fatalf("empty run = %+v", res)
	}
	f := newFake()
	res := Run(context.Background(), func(context.Context) (Session, error) { return f, nil }, fakeJobs("a.php"), 0, func(Result) {})
	if len(res) != 1 || res[0].Err != nil {
		t.Fatalf("results = %+v", res)
	}
}
