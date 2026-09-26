//go:build windows

package ptys

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func shell() string {
	if c := os.Getenv("COMSPEC"); c != "" {
		return c
	}
	return "cmd.exe"
}

// readUntil junta a saida ate want aparecer ou o prazo acabar.
func readUntil(t *testing.T, m *Manager, id int64, want string, deadline time.Time) string {
	var out strings.Builder
	for !strings.Contains(out.String(), want) && time.Now().Before(deadline) {
		data, err := m.Read(context.Background(), id, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("Read: %v (saida ate aqui: %q)", err, out.String())
		}
		out.Write(data)
	}
	return out.String()
}

// Um cmd.exe de verdade num pseudoconsole (ConPTY): roda na pasta pedida,
// executa um comando, redimensiona e termina com exit.
func TestRealShell(t *testing.T) {
	m := NewManager(Start)
	dir := t.TempDir()
	id, err := m.Open(shell(), dir, 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	// a linha digitada nao contem "ok_42"; so a saida do comando contem
	if err := m.Write(id, []byte("for /f %i in ('set /a 6*7') do @echo ok_%i\r")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := readUntil(t, m, id, "ok_42", deadline)
	if !strings.Contains(out, "ok_42") {
		t.Fatalf("saida do shell: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), strings.ToLower(dir)) {
		t.Fatalf("o prompt deveria mostrar a pasta %q: %q", dir, out)
	}
	if err := m.Resize(id, 120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	m.Write(id, []byte("exit\r"))
	for time.Now().Before(deadline) {
		if _, err := m.Read(context.Background(), id, 500*time.Millisecond); errors.Is(err, ErrExited) {
			m.Close(id)
			return
		}
	}
	t.Fatal("o shell nao terminou depois de exit")
}

// Kill derruba o processo e destrava quem esta lendo: a leitura acaba com
// EOF (ou erro) em vez de ficar presa no pipe do pseudoconsole.
func TestKillEndsProcessAndUnblocksRead(t *testing.T) {
	p, err := Start(shell(), t.TempDir(), 80, 24)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := p.Read(buf); err != nil {
				done <- err
				return
			}
		}
	}()
	time.Sleep(300 * time.Millisecond) // o cmd chega a desenhar o prompt
	if err := p.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	select {
	case err := <-done:
		if err != io.EOF && err == nil {
			t.Fatalf("leitura depois de Kill: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read continuou bloqueado depois de Kill")
	}
	if err := p.Kill(); err != nil {
		t.Fatalf("Kill de novo deve ser inofensivo: %v", err)
	}
}

// A pasta inexistente e o erro do CreateProcess, nao um terminal aberto.
func TestBadCwdFails(t *testing.T) {
	if _, err := Start(shell(), `C:\nao\existe\mesmo`, 80, 24); err == nil {
		t.Fatal("Start numa pasta inexistente deveria falhar")
	}
}
