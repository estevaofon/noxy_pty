//go:build windows

package ptys

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winProc e um processo num pseudoconsole do Windows (ConPTY, Windows 10
// 1809 ou mais novo). O conhost fica entre o programa e dois pipes: le a
// entrada de um e escreve a saida, ja em UTF-8 com sequencias VT, no outro.
// O pipe de saida so chega ao fim no ClosePseudoConsole, entao wait espera o
// processo terminar e fecha o pseudoconsole para o leitor ver EOF.
type winProc struct {
	hpc  windows.Handle // o pseudoconsole
	proc windows.Handle // o processo
	in   *os.File       // nossa ponta do pipe de entrada (escrita)
	out  *os.File       // nossa ponta do pipe de saida (leitura)

	mu       sync.Mutex
	released bool
	outOnce  sync.Once
}

// Start abre cmd num pseudoconsole com TERM=xterm-256color, no diretorio cwd.
// Em Windows sem ConPTY o erro vem do proprio CreatePseudoConsole.
func Start(cmd, cwd string, cols, rows int) (Proc, error) {
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe: %w", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, fmt.Errorf("CreatePipe: %w", err)
	}
	var hpc windows.Handle
	err := windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inR, outW, 0, &hpc)
	// o pseudoconsole guarda copias das pontas dele; as nossas fecham ja
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	proc, err := spawn(cmd, cwd, hpc)
	if err != nil {
		windows.ClosePseudoConsole(hpc)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}
	p := &winProc{
		hpc:  hpc,
		proc: proc,
		in:   os.NewFile(uintptr(inW), "conpty-in"),
		out:  os.NewFile(uintptr(outR), "conpty-out"),
	}
	go p.wait()
	return p, nil
}

// spawn inicia cmd em cwd ligado ao pseudoconsole hpc e devolve o handle do
// processo. O atributo PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE leva o proprio
// handle como valor (nao um ponteiro para ele), como a API pede; a leitura
// do handle como ponteiro e o jeito de dizer isso sem o vet reclamar da
// conversao uintptr -> unsafe.Pointer.
func spawn(cmd, cwd string, hpc windows.Handle) (windows.Handle, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	defer attrs.Delete()
	value := *(*unsafe.Pointer)(unsafe.Pointer(&hpc))
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, value, unsafe.Sizeof(hpc)); err != nil {
		return 0, fmt.Errorf("UpdateProcThreadAttribute: %w", err)
	}
	si := new(windows.StartupInfoEx)
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.ProcThreadAttributeList = attrs.List()
	// Sem isto o filho herda os handles padrao do pai mesmo com
	// bInheritHandles=false (o kernel os copia para processos de console); com
	// o stdin e o stdout do pai redirecionados (um pipe, como nos testes e na
	// extensao), o cmd escreveria neles e sairia no EOF do stdin. Com
	// STARTF_USESTDHANDLES e os tres handles zerados, o filho recebe os do
	// pseudoconsole ao se conectar a ele.
	si.Flags = windows.STARTF_USESTDHANDLES
	cmdLine, err := windows.UTF16PtrFromString(windows.EscapeArg(cmd))
	if err != nil {
		return 0, err
	}
	dir, err := windows.UTF16PtrFromString(cwd)
	if err != nil {
		return 0, err
	}
	env := environmentBlock("TERM=xterm-256color", "COLORTERM=truecolor")
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(nil, cmdLine, nil, nil, false, flags, &env[0], dir, &si.StartupInfo, &pi); err != nil {
		return 0, fmt.Errorf("CreateProcess %s: %w", cmd, err)
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}

// environmentBlock e o ambiente atual mais extra, no formato do
// CreateProcess: "K=V\0" repetido, em UTF-16, com um "\0" final.
func environmentBlock(extra ...string) []uint16 {
	var b []uint16
	for _, kv := range append(os.Environ(), extra...) {
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	return append(b, 0)
}

// wait espera o processo terminar e solta o pseudoconsole: e isso que faz o
// pipe de saida chegar ao fim, com o que o programa ainda tinha escrito.
func (p *winProc) wait() {
	windows.WaitForSingleObject(p.proc, windows.INFINITE)
	p.release()
	windows.CloseHandle(p.proc)
}

// release termina o processo (se ainda roda), fecha a entrada e o
// pseudoconsole. O conhost entrega o que falta ao pipe de saida e fecha a
// ponta dele; e preciso haver alguem lendo (o pump do Manager), senao o
// ClosePseudoConsole espera. Idempotente.
func (p *winProc) release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released {
		return
	}
	p.released = true
	windows.TerminateProcess(p.proc, 1)
	p.in.Close()
	windows.ClosePseudoConsole(p.hpc)
}

// Read le a saida; o fim do pipe (o pseudoconsole fechou) e io.EOF, e ai a
// nossa ponta e fechada.
func (p *winProc) Read(b []byte) (int, error) {
	n, err := p.out.Read(b)
	if err != nil {
		if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, os.ErrClosed) {
			err = io.EOF
		}
		p.outOnce.Do(func() { p.out.Close() })
	}
	return n, err
}

func (p *winProc) Write(b []byte) (int, error) { return p.in.Write(b) }

func (p *winProc) Resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.released {
		return errors.New("terminal closed")
	}
	return windows.ResizePseudoConsole(p.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

// Kill termina o processo e fecha o pseudoconsole, o que encerra tambem o
// que o programa abriu nele (o conhost manda CTRL_CLOSE_EVENT a todos).
func (p *winProc) Kill() error {
	p.release()
	return nil
}
