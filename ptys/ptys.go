// Package ptys guarda os terminais abertos de um processo da extensao: cada
// um tem um leitor em segundo plano que junta a saida num buffer, e Read
// espera ate haver dados, o tempo acabar ou o processo terminar. O processo
// real (pty) vem de um Starter injetado; os testes usam um falso.
package ptys

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Proc e o processo num terminal: ler e escrever na pty, redimensionar, matar.
type Proc interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Resize(cols, rows int) error
	Kill() error
}

// Starter abre cmd numa pty com o diretorio e o tamanho dados.
type Starter func(cmd, cwd string, cols, rows int) (Proc, error)

var (
	ErrUnknown = errors.New("unknown terminal")
	ErrExited  = errors.New("terminal exited")
)

const maxRead = 64 * 1024

type term struct {
	proc   Proc
	mu     sync.Mutex
	buf    []byte
	exited bool
	signal chan struct{} // recebe um aviso quando chegam dados ou o fim
}

type Manager struct {
	start Starter
	mu    sync.Mutex
	next  int64
	terms map[int64]*term
}

func NewManager(start Starter) *Manager {
	return &Manager{start: start, terms: map[int64]*term{}}
}

func (m *Manager) get(id int64) (*term, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.terms[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrUnknown, id)
	}
	return t, nil
}

// Open abre um terminal e devolve o id.
func (m *Manager) Open(cmd, cwd string, cols, rows int) (int64, error) {
	if cols <= 0 || rows <= 0 {
		return 0, fmt.Errorf("cols and rows must be positive, got %dx%d", cols, rows)
	}
	p, err := m.start(cmd, cwd, cols, rows)
	if err != nil {
		return 0, err
	}
	t := &term{proc: p, signal: make(chan struct{}, 1)}
	m.mu.Lock()
	m.next++
	id := m.next
	m.terms[id] = t
	m.mu.Unlock()
	go t.pump()
	return id, nil
}

// pump le do processo ate o fim e junta no buffer.
func (t *term) pump() {
	chunk := make([]byte, 32*1024)
	for {
		n, err := t.proc.Read(chunk)
		t.mu.Lock()
		if n > 0 {
			t.buf = append(t.buf, chunk[:n]...)
		}
		if err != nil {
			t.exited = true
		}
		t.mu.Unlock()
		select {
		case t.signal <- struct{}{}:
		default:
		}
		if err != nil {
			return
		}
	}
}

// take tira ate maxRead bytes do buffer; exited so quando nao sobra nada.
func (t *term) take() ([]byte, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buf) > 0 {
		n := len(t.buf)
		if n > maxRead {
			n = maxRead
		}
		out := append([]byte(nil), t.buf[:n]...)
		t.buf = t.buf[n:]
		return out, false
	}
	return nil, t.exited
}

// Read espera ate timeout por dados; devolve vazio no timeout e ErrExited
// quando o processo terminou e nao ha mais dados.
func (m *Manager) Read(ctx context.Context, id int64, timeout time.Duration) ([]byte, error) {
	t, err := m.get(id)
	if err != nil {
		return nil, err
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		data, exited := t.take()
		if len(data) > 0 {
			return data, nil
		}
		if exited {
			return nil, ErrExited
		}
		select {
		case <-t.signal:
		case <-deadline.C:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (m *Manager) Write(id int64, data []byte) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	_, err = t.proc.Write(data)
	return err
}

func (m *Manager) Resize(id int64, cols, rows int) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	return t.proc.Resize(cols, rows)
}

// Close mata o processo e esquece o terminal; id desconhecido e no-op.
func (m *Manager) Close(id int64) error {
	m.mu.Lock()
	t, ok := m.terms[id]
	delete(m.terms, id)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return t.proc.Kill()
}

// CloseAll mata todos os terminais (fim do processo da extensao).
func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]int64, 0, len(m.terms))
	for id := range m.terms {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Close(id)
	}
}
