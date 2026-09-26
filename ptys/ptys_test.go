package ptys

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeProc e um processo sem sistema: o que o teste escreve em out aparece
// na leitura; Kill fecha out (fim do processo).
type fakeProc struct {
	out     *io.PipeReader
	outW    *io.PipeWriter
	mu      sync.Mutex
	written []byte
	size    [2]int
	killed  bool
}

func newFake() *fakeProc {
	r, w := io.Pipe()
	return &fakeProc{out: r, outW: w}
}

func (f *fakeProc) Read(p []byte) (int, error) { return f.out.Read(p) }
func (f *fakeProc) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p...)
	return len(p), nil
}
func (f *fakeProc) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.size = [2]int{cols, rows}
	return nil
}
func (f *fakeProc) Kill() error {
	f.mu.Lock()
	f.killed = true
	f.mu.Unlock()
	return f.outW.Close()
}

func managerWith(f *fakeProc) *Manager {
	return NewManager(func(cmd, cwd string, cols, rows int) (Proc, error) {
		if cmd == "falha" {
			return nil, errors.New("nao abriu")
		}
		f.Resize(cols, rows)
		return f, nil
	})
}

func TestOpenReadWriteResizeClose(t *testing.T) {
	f := newFake()
	m := managerWith(f)
	id, err := m.Open("/bin/sh", "/tmp", 80, 24)
	if err != nil || id <= 0 {
		t.Fatalf("Open: %d %v", id, err)
	}
	if f.size != [2]int{80, 24} {
		t.Fatalf("tamanho inicial: %v", f.size)
	}
	go f.outW.Write([]byte("ola"))
	got, err := m.Read(context.Background(), id, time.Second)
	if err != nil || string(got) != "ola" {
		t.Fatalf("Read: %q %v", got, err)
	}
	if err := m.Write(id, []byte("ls\n")); err != nil || string(f.written) != "ls\n" {
		t.Fatalf("Write: %v %q", err, f.written)
	}
	if err := m.Resize(id, 100, 30); err != nil || f.size != [2]int{100, 30} {
		t.Fatalf("Resize: %v %v", err, f.size)
	}
	if err := m.Close(id); err != nil || !f.killed {
		t.Fatalf("Close: %v %v", err, f.killed)
	}
	if err := m.Close(id); err != nil {
		t.Fatalf("Close de novo deve ser idempotente: %v", err)
	}
	if err := m.Write(id, []byte("x")); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Write depois de Close: %v", err)
	}
}

func TestReadTimesOutEmpty(t *testing.T) {
	f := newFake()
	m := managerWith(f)
	id, _ := m.Open("/bin/sh", "/", 80, 24)
	start := time.Now()
	got, err := m.Read(context.Background(), id, 50*time.Millisecond)
	if err != nil || len(got) != 0 || time.Since(start) < 40*time.Millisecond {
		t.Fatalf("timeout: %q %v %v", got, err, time.Since(start))
	}
	m.Close(id)
}

func TestReadDrainsThenReportsExit(t *testing.T) {
	f := newFake()
	m := managerWith(f)
	id, _ := m.Open("/bin/sh", "/", 80, 24)
	f.outW.Write([]byte("fim"))
	f.outW.Close() // o processo terminou depois de escrever
	got, err := m.Read(context.Background(), id, time.Second)
	if err != nil || string(got) != "fim" {
		t.Fatalf("dados antes do fim: %q %v", got, err)
	}
	if _, err := m.Read(context.Background(), id, time.Second); !errors.Is(err, ErrExited) {
		t.Fatalf("depois dos dados, fim do processo: %v", err)
	}
}

func TestReadHonoursContext(t *testing.T) {
	f := newFake()
	m := managerWith(f)
	id, _ := m.Open("/bin/sh", "/", 80, 24)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := m.Read(ctx, id, 10*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read com contexto cancelado: %v", err)
	}
	m.Close(id)
}

func TestOpenFailureAndUnknownIDs(t *testing.T) {
	m := managerWith(newFake())
	if _, err := m.Open("falha", "/", 80, 24); err == nil {
		t.Fatal("Open que falha deve devolver erro")
	}
	if _, err := m.Read(context.Background(), 99, time.Millisecond); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Read de id desconhecido: %v", err)
	}
	if err := m.Resize(99, 1, 1); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Resize de id desconhecido: %v", err)
	}
	if err := m.Close(99); err != nil {
		t.Fatalf("Close de id desconhecido e no-op: %v", err)
	}
	if _, err := m.Open("/bin/sh", "/", 0, 24); err == nil {
		t.Fatal("tamanho nao positivo deve ser erro")
	}
}

func TestCloseAllKillsEverything(t *testing.T) {
	a, b := newFake(), newFake()
	n := 0
	m := NewManager(func(cmd, cwd string, cols, rows int) (Proc, error) {
		n++
		if n == 1 {
			return a, nil
		}
		return b, nil
	})
	m.Open("/bin/sh", "/", 80, 24)
	m.Open("/bin/sh", "/", 80, 24)
	m.CloseAll()
	if !a.killed || !b.killed {
		t.Fatalf("CloseAll: %v %v", a.killed, b.killed)
	}
}
