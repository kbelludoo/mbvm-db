// Package competitorvalidation compares bounded code-navigation tasks using
// existing persistent tool processes. It does not assume MBVM-DB must win.
package competitorvalidation

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type wireMessage struct {
	Direction string          `json:"direction"`
	Payload   json.RawMessage `json:"payload"`
}

type process struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	out   *bufio.Reader
	log   *os.File
	trace []wireMessage
}

func startProcess(ctx context.Context, binary, root, logPath string, args ...string) (*process, error) {
	log, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create stderr log: %w", err)
	}
	p := &process{cmd: exec.CommandContext(ctx, binary, args...), log: log}
	p.cmd.Dir, p.cmd.Stderr = root, log
	p.in, err = p.cmd.StdinPipe()
	if err == nil {
		var out io.ReadCloser
		out, err = p.cmd.StdoutPipe()
		p.out = bufio.NewReader(out)
	}
	if err == nil {
		err = p.cmd.Start()
	}
	if err != nil {
		log.Close()
		return nil, fmt.Errorf("start %s: %w", binary, err)
	}
	return p, nil
}

func (p *process) close(path string) error {
	_ = p.in.Close()
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
	_ = p.log.Close()
	b, err := json.MarshalIndent(p.trace, "", "  ")
	if err != nil {
		return fmt.Errorf("encode wire trace: %w", err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		return fmt.Errorf("save wire trace: %w", err)
	}
	return nil
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type lspClient struct {
	*process
	next int
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read LSP header: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(key, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("parse LSP length: %w", err)
			}
		}
	}
	if length < 0 || length > 32<<20 {
		return nil, fmt.Errorf("invalid LSP frame length %d", length)
	}
	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read LSP body: %w", err)
	}
	return b, nil
}

func (c *lspClient) send(m rpcMessage) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode LSP message: %w", err)
	}
	if _, err = fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(b), b); err != nil {
		return fmt.Errorf("send LSP: %w", err)
	}
	c.trace = append(c.trace, wireMessage{"send", b})
	return nil
}

func (c *lspClient) notify(method string, params json.RawMessage) error {
	return c.send(rpcMessage{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *lspClient) call(method string, params json.RawMessage) (json.RawMessage, error) {
	c.next++
	id := json.RawMessage(strconv.Itoa(c.next))
	if err := c.send(rpcMessage{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	for {
		b, err := readFrame(c.out)
		if err != nil {
			return nil, err
		}
		c.trace = append(c.trace, wireMessage{"receive", b})
		var m rpcMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("decode LSP response: %w", err)
		}
		if m.Method != "" {
			if len(m.ID) > 0 {
				// Capabilities do not advertise dynamic configuration. Still
				// acknowledge progress/registration requests to avoid deadlock.
				result := json.RawMessage("null")
				if m.Method == "workspace/configuration" {
					result = json.RawMessage("[null]")
				}
				if err := c.send(rpcMessage{JSONRPC: "2.0", ID: m.ID, Result: result}); err != nil {
					return nil, err
				}
			}
			continue
		}
		if string(m.ID) != string(id) {
			return nil, fmt.Errorf("unexpected RPC id %s, want %s", m.ID, id)
		}
		if m.Error != nil {
			return nil, fmt.Errorf("LSP %s error %d: %s", method, m.Error.Code, m.Error.Message)
		}
		return m.Result, nil
	}
}
