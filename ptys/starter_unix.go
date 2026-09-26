//go:build !windows

package ptys

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type unixProc struct {
	f   *os.File
	cmd *exec.Cmd
}

// Start abre cmd numa pty com TERM=xterm-256color, no diretorio cwd.
func Start(cmd, cwd string, cols, rows int) (Proc, error) {
	c := exec.Command(cmd)
	c.Dir = cwd
	c.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, err := pty.StartWithSize(c, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	go c.Wait() // recolhe o processo quando ele terminar
	return &unixProc{f: f, cmd: c}, nil
}

func (p *unixProc) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *unixProc) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *unixProc) Resize(cols, rows int) error {
	return pty.Setsize(p.f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// Kill manda SIGHUP (o que um terminal fechado manda) e fecha a pty.
func (p *unixProc) Kill() error {
	if p.cmd.Process != nil {
		p.cmd.Process.Signal(syscall.SIGHUP)
	}
	return p.f.Close()
}
