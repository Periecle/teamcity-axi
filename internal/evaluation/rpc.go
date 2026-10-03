package evaluation

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

type rpcReply struct {
	value Object
	err   error
}

const modelProtocolCaptureLimit = 64 * 1024 * 1024

// Count the entire stream, including notifications that are not retained as events.
type protocolCaptureReader struct {
	reader io.Reader
	total  int
}

func (capture *protocolCaptureReader) Read(p []byte) (int, error) {
	n, err := capture.reader.Read(p)
	capture.total += n
	if capture.total > modelProtocolCaptureLimit {
		return 0, errors.New("Model runtime exceeded the aggregate protocol capture limit")
	}
	return n, err
}

type rpcClient struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	mu       sync.Mutex
	writeMu  sync.Mutex
	pending  map[string]chan rpcReply
	nextID   int
	events   []Object
	finished bool
	complete chan rpcReply
	closed   chan struct{}
	tools    sync.WaitGroup
	tool     func(context.Context, Object) (Object, error)
}

func startRPC(ctx context.Context, binary string, args, env []string, cwd string, tool func(context.Context, Object) (Object, error)) (*rpcClient, error) {
	life, cancel := context.WithCancelCause(ctx)
	cmd := exec.Command(binary, args...)
	cmd.Env, cmd.Dir = env, cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 400 * time.Millisecond
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel(err)
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel(err)
		return nil, err
	}
	// stderr is deliberately captured by a bounded sink and never included in errors.
	stderr := &captureChannel{budget: &captureBudget{limit: 65536, stop: cancel}}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel(err)
		return nil, err
	}
	client := &rpcClient{ctx: life, cancel: cancel, cmd: cmd, stdin: input, pending: map[string]chan rpcReply{}, nextID: 1, events: []Object{}, complete: make(chan rpcReply, 1), closed: make(chan struct{}), tool: tool}
	go client.read(output)
	go func() {
		select {
		case <-life.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-client.closed:
		}
	}()
	return client, nil
}

func (client *rpcClient) fail(err error) {
	client.cancel(err)
	client.mu.Lock()
	for key, channel := range client.pending {
		channel <- rpcReply{err: err}
		delete(client.pending, key)
	}
	if !client.finished {
		client.finished = true
		client.complete <- rpcReply{err: err}
	}
	client.mu.Unlock()
}

func (client *rpcClient) send(message Object) error {
	bytes, err := json.Marshal(message)
	if err != nil {
		return err
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	_, err = client.stdin.Write(append(bytes, '\n'))
	return err
}

func (client *rpcClient) request(method string, params Object) (Object, error) {
	client.mu.Lock()
	id := client.nextID
	client.nextID++
	key := fmt.Sprint(id)
	channel := make(chan rpcReply, 1)
	client.pending[key] = channel
	client.mu.Unlock()
	if err := client.send(Object{"id": id, "method": method, "params": params}); err != nil {
		client.fail(err)
		return nil, err
	}
	select {
	case reply := <-channel:
		return reply.value, reply.err
	case <-client.ctx.Done():
		return nil, context.Cause(client.ctx)
	}
}

func (client *rpcClient) read(output io.Reader) {
	defer close(client.closed)
	scanner := bufio.NewScanner(&protocolCaptureReader{reader: output})
	scanner.Buffer(make([]byte, 65536), 2097152)
	for scanner.Scan() {
		var message Object
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			client.fail(errors.New("Invalid model runtime protocol document"))
			break
		}
		method := axi.Str(message, "method")
		id, hasID := message["id"]
		if hasID && method == "" {
			client.mu.Lock()
			channel := client.pending[valueString(id)]
			delete(client.pending, valueString(id))
			client.mu.Unlock()
			if channel != nil {
				if message["error"] != nil {
					channel <- rpcReply{err: errors.New("Model runtime rejected a protocol request")}
				} else {
					channel <- rpcReply{value: axi.Obj(message["result"])}
				}
			}
			continue
		}
		params := axi.Obj(message["params"])
		if method == "item/started" || method == "item/completed" {
			item := axi.Obj(params["item"])
			kind := axi.Str(item, "type")
			if !has([]string{"userMessage", "reasoning", "agentMessage", "dynamicToolCall"}, kind) || (kind == "dynamicToolCall" && axi.Str(item, "tool") != "evaluation_shell") {
				client.fail(errors.New("Unexpected built-in tool activity invalidates session isolation"))
				break
			}
		}
		if method == "item/tool/call" {
			if !hasID {
				client.fail(errors.New("Dynamic tool request has no protocol identity"))
				break
			}
			client.tools.Add(1)
			go func(message Object) {
				defer client.tools.Done()
				result, err := client.tool(client.ctx, message)
				if err != nil {
					client.fail(err)
					return
				}
				if err := client.send(Object{"id": message["id"], "result": result}); err != nil {
					client.fail(err)
				}
			}(message)
		} else if hasID {
			client.fail(fmt.Errorf("Unexpected server request: %s", method))
			break
		}
		if has([]string{"item/started", "item/completed", "turn/completed", "thread/tokenUsage/updated", "error"}, method) {
			client.mu.Lock()
			client.events = append(client.events, message)
			if method == "turn/completed" && !client.finished {
				client.finished = true
				client.complete <- rpcReply{value: axi.Obj(params["turn"])}
			}
			client.mu.Unlock()
		}
	}
	if err := scanner.Err(); err != nil {
		client.fail(errors.New("Model runtime output exceeded the bounded protocol capture or failed"))
	}
	client.mu.Lock()
	finished := client.finished
	client.mu.Unlock()
	if !finished {
		client.fail(errors.New("Model runtime exited before completion"))
	}
}

func (client *rpcClient) awaitTurn() (Object, error) {
	select {
	case reply := <-client.complete:
		return reply.value, reply.err
	case <-client.ctx.Done():
		return nil, context.Cause(client.ctx)
	}
}

func (client *rpcClient) Events() []Object {
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]Object{}, client.events...)
}

func (client *rpcClient) Close() {
	client.cancel(errors.New("Model runtime disposed"))
	_ = client.stdin.Close()
	_ = syscall.Kill(-client.cmd.Process.Pid, syscall.SIGTERM)
	timer := time.AfterFunc(time.Second, func() { _ = syscall.Kill(-client.cmd.Process.Pid, syscall.SIGKILL) })
	_ = client.cmd.Wait()
	timer.Stop()
	_ = syscall.Kill(-client.cmd.Process.Pid, syscall.SIGKILL)
	<-client.closed
	client.tools.Wait()
}
