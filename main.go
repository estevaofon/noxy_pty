// noxy_pty — terminais (pty) para programas Noxy, empacotado como extensao
// por processo (kind = "process" em noxy_ext.toml). Os dados cruzam em
// base64: bytes de terminal nao sao UTF-8 garantido.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/noxylang/noxy/sdk/noxyplugin"

	"github.com/noxylang/noxy_pty/ptys"
)

func main() {
	m := ptys.NewManager(ptys.Start)
	p := noxyplugin.New()
	p.Handle("pty_open", noxyplugin.Func4(func(ctx context.Context, cmd, cwd string, cols, rows int64) (int64, error) {
		return m.Open(cmd, cwd, int(cols), int(rows))
	}))
	p.Handle("pty_read", noxyplugin.Func2(func(ctx context.Context, id, timeoutMs int64) (string, error) {
		data, err := m.Read(ctx, id, time.Duration(timeoutMs)*time.Millisecond)
		if errors.Is(err, ptys.ErrExited) {
			return "", fmt.Errorf("pty %d exited", id)
		}
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(data), nil
	}))
	p.Handle("pty_write", noxyplugin.Func2(func(ctx context.Context, id int64, data string) (any, error) {
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("pty_write: data is not base64: %v", err)
		}
		return nil, m.Write(id, raw)
	}))
	p.Handle("pty_resize", noxyplugin.Func3(func(ctx context.Context, id, cols, rows int64) (any, error) {
		return nil, m.Resize(id, int(cols), int(rows))
	}))
	p.Handle("pty_close", noxyplugin.Func1(func(ctx context.Context, id int64) (any, error) {
		return nil, m.Close(id)
	}))
	defer m.CloseAll()
	p.Main() // serve stdin/stdout; sai quando o host fecha o stdin
}
