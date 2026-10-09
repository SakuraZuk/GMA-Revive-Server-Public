// Package nativeengine 驱动隔离的CPython2原生战斗进程，不访问账号或数据库。
package nativeengine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

const MaxPacketBytes = 8 << 20

type Config struct {
	Python, Worker, Directory string
}

type Rejection struct{ Message string }

func (e *Rejection) Error() string { return "原生引擎拒绝输入：" + e.Message }

type packet struct {
	Value     json.RawMessage `json:"value"`
	Error     string          `json:"error"`
	ErrorCode string          `json:"error_code"`
	Message   string          `json:"message"`
}

type response struct {
	packet packet
	err    error
}

type Process struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	input    io.WriteCloser
	output   chan response
	finished chan error
	stop     chan struct{}
	closed   bool
}

// 每场一个进程；原生stdout专供有界JSON协议，业务代码日志走stderr。
func Open(ctx context.Context, config Config) (*Process, error) {
	if config.Python == "" || config.Worker == "" || config.Directory == "" {
		return nil, errors.New("原生引擎需要明确的解释器、worker和资源目录")
	}
	cmd := exec.Command(config.Python, "-B", "-u", config.Worker)
	cmd.Dir = config.Directory
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	// 原生异常已在结构化回复中提供；不把子进程的无限日志积存在内存。
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, err
	}
	p := &Process{cmd: cmd, input: input, output: make(chan response, 1), finished: make(chan error, 1), stop: make(chan struct{})}
	go func() {
		defer close(p.output)
		send := func(r response) bool {
			select {
			case p.output <- r:
				return true
			case <-p.stop:
				return false
			}
		}
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64*1024), MaxPacketBytes)
		for scanner.Scan() {
			var value packet
			decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			if err := decoder.Decode(&value); err != nil {
				send(response{err: fmt.Errorf("原生进程返回非法JSON：%w", err)})
				return
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				send(response{err: errors.New("原生进程一行回复包含多余JSON内容")})
				return
			}
			if !send(response{packet: value}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			send(response{err: fmt.Errorf("原生进程协议超限或读取失败：%w", err)})
		}
	}()
	go func() { p.finished <- cmd.Wait() }()
	if _, err = p.Request(ctx, map[string]any{"operation": "boot", "modules": []string{}}); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

// 所有请求按一个FIFO串行执行；超时或坏回复立即废弃进程，不继续使用半执行状态。
func (p *Process) Request(ctx context.Context, request map[string]any) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("原生战斗进程已关闭")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(request)
	if err != nil || len(body) >= MaxPacketBytes {
		return nil, errors.New("原生战斗请求JSON无效或超过保护上限")
	}
	written := make(chan error, 1)
	go func() { _, err := p.input.Write(append(body, '\n')); written <- err }()
	select {
	case <-ctx.Done():
		p.closeUnsafe()
		return nil, ctx.Err()
	case err := <-written:
		if err != nil {
			p.closeUnsafe()
			return nil, err
		}
	}
	select {
	case <-ctx.Done():
		p.closeUnsafe()
		return nil, ctx.Err()
	case reply, ok := <-p.output:
		if !ok {
			p.closeUnsafe()
			return nil, errors.New("原生战斗进程提前退出")
		}
		if reply.err != nil {
			p.closeUnsafe()
			return nil, reply.err
		}
		if reply.packet.ErrorCode == "command_rejected" {
			return nil, &Rejection{reply.packet.Message}
		}
		if reply.packet.Error != "" || reply.packet.Value == nil {
			p.closeUnsafe()
			return nil, fmt.Errorf("原生战斗运行失败：%s\n%s", reply.packet.Message, reply.packet.Error)
		}
		return reply.packet.Value, nil
	}
}

func (p *Process) closeUnsafe() {
	if p.closed {
		return
	}
	p.closed = true
	close(p.stop)
	_ = p.input.Close()
	_ = p.cmd.Process.Kill()
	<-p.finished // 回收自己启动的进程，不能留下僵尸或影响其它比赛。
}

func (p *Process) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeUnsafe()
}
