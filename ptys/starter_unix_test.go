//go:build !windows

package ptys

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Um shell de verdade numa pty: escreve um comando, le a saida, fecha.
func TestRealShell(t *testing.T) {
	m := NewManager(Start)
	id, err := m.Open("/bin/sh", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := m.Write(id, []byte("echo ok_$((1+1))\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var out strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "ok_2") && time.Now().Before(deadline) {
		data, err := m.Read(context.Background(), id, 500*time.Millisecond)
		if err != nil {
			t.Fatalf("Read: %v (saida ate aqui: %q)", err, out.String())
		}
		out.Write(data)
	}
	if !strings.Contains(out.String(), "ok_2") {
		t.Fatalf("saida do shell: %q", out.String())
	}
	if err := m.Resize(id, 120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	m.Write(id, []byte("exit\n"))
	for time.Now().Before(deadline) {
		if _, err := m.Read(context.Background(), id, 500*time.Millisecond); err == ErrExited {
			m.Close(id)
			return
		}
	}
	t.Fatal("o shell nao terminou depois de exit")
}
