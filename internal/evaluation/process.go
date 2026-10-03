package evaluation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Call struct {
	Argv            []string `json:"argv,omitempty"`
	Code            int      `json:"code"`
	Signal          any      `json:"signal"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	WallTimeMS      float64  `json:"wallTimeMs"`
	CaptureLimitHit bool     `json:"captureLimitHit,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	RunID           string   `json:"runId,omitempty"`
	CallID          string   `json:"callId,omitempty"`
	Command         string   `json:"command,omitempty"`
	Body            Object   `json:"-"`
}

type captureBudget struct {
	mu    sync.Mutex
	total int
	limit int
	stop  context.CancelCauseFunc
	hit   bool
}

type captureChannel struct {
	budget *captureBudget
	buffer bytes.Buffer
}

func (stream *captureChannel) Write(p []byte) (int, error) {
	stream.budget.mu.Lock()
	defer stream.budget.mu.Unlock()
	stream.budget.total += len(p)
	if stream.budget.total > stream.budget.limit {
		stream.budget.hit = true
		stream.budget.stop(errors.New("Evaluation capture limit exceeded"))
		return 0, io.ErrShortBuffer
	}
	return stream.buffer.Write(p)
}

// Execute uses a bounded capture and a killable process group for every evaluation tool.
func Execute(ctx context.Context, binary string, argv []string, env []string, cwd string, ceiling int) (Call, error) {
	start := time.Now()
	child, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	budget := &captureBudget{limit: ceiling, stop: cancel}
	out, stderr := &captureChannel{budget: budget}, &captureChannel{budget: budget}
	cmd := exec.Command(binary, argv...)
	cmd.Env, cmd.Dir, cmd.Stdout, cmd.Stderr = env, cwd, out, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 400 * time.Millisecond
	if child.Err() != nil {
		return Call{}, context.Cause(child)
	}
	if err := cmd.Start(); err != nil {
		return Call{}, err
	}
	done, cleaned := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(cleaned)
		select {
		case <-done:
			return
		case <-child.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	err := cmd.Wait()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	close(done)
	<-cleaned
	result := Call{Argv: append([]string{}, argv...), Code: cmd.ProcessState.ExitCode(), Stdout: out.buffer.String(), Stderr: stderr.buffer.String(), WallTimeMS: float64(time.Since(start)) / float64(time.Millisecond), CaptureLimitHit: budget.hit}
	if state, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && state.Signaled() {
		result.Signal = state.Signal().String()
	}
	if cause := context.Cause(child); cause != nil {
		return result, cause
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return result, err
		}
	}
	return result, nil
}
