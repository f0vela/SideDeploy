package deploy

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"
)

// Job is one file to upload or one remote file to delete.
type Job struct {
	Rel    string
	Local  string
	Remote string
	Delete bool
}

// Result is what happened to a job.
type Result struct {
	Job
	Err error
}

// Session is one connection to the server.
type Session interface {
	Upload(ctx context.Context, local, remote string) error
	MkdirAll(ctx context.Context, dir string) error
	Delete(ctx context.Context, remote string) error
	Close() error
}

// Opener connects; each worker calls it once.
type Opener func(ctx context.Context) (Session, error)

// Run works through jobs with the given number of connections and reports
// each result as it ends. Canceling ctx stops the work; the jobs not started
// return no result.
func Run(ctx context.Context, open Opener, jobs []Job, workers int, report func(Result)) []Result {
	if len(jobs) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	work := make(chan Job)
	var mu sync.Mutex
	var results []Result
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := open(ctx)
			if err != nil {
				for job := range work {
					r := Result{Job: job, Err: err}
					mu.Lock()
					results = append(results, r)
					mu.Unlock()
					report(r)
				}
				return
			}
			// Closing the session is what stops a transfer already running.
			stopped := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					session.Close()
				case <-stopped:
				}
			}()

			created := make(map[string]bool)
			for job := range work {
				if ctx.Err() != nil {
					break
				}
				r := Result{Job: job, Err: runJob(ctx, session, job, created)}
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
				report(r)
			}
			close(stopped)
			session.Close()
		}()
	}

feed:
	for _, job := range jobs {
		select {
		case work <- job:
		case <-ctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()
	return results
}

func runJob(ctx context.Context, s Session, job Job, created map[string]bool) error {
	if job.Delete {
		return retry(ctx, func() error { return s.Delete(ctx, job.Remote) })
	}
	dir := path.Dir(job.Remote)
	if !created[dir] {
		if err := s.MkdirAll(ctx, dir); err != nil {
			return fmt.Errorf("crear %s: %w", dir, err)
		}
		created[dir] = true
	}
	return retry(ctx, func() error { return s.Upload(ctx, job.Local, job.Remote) })
}

// retry gives a failing transfer one more chance, for the servers that drop
// a data connection now and then.
func retry(ctx context.Context, fn func() error) error {
	err := fn()
	if err == nil || ctx.Err() != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return err
	case <-time.After(400 * time.Millisecond):
	}
	if err := fn(); err != nil {
		return err
	}
	return nil
}
