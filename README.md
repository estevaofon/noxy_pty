# noxy_pty

Terminais de verdade (pty) para programas [Noxy](https://github.com/noxylang/noxy),
como extensão por processo: [creack/pty](https://github.com/creack/pty) no
Linux e no macOS, ConPTY (pseudoconsole) no Windows. Feita para o terminal do
[Noxy Editor](https://github.com/noxylang/Noxy-Editor); serve para qualquer
programa Noxy que precise rodar um shell ou um programa interativo e ler e
escrever nele.

## Instalação

    noxy --get github.com/noxylang/noxy_pty

Binários para Linux (amd64, arm64), macOS (Intel, Apple Silicon) e Windows
(amd64). Requer Noxy 0.25.0 ou mais novo; no Windows, a versão 10 1809 ou
mais nova (é quando o ConPTY apareceu). Num Windows mais antigo `open` falha
com o erro do `CreatePseudoConsole`.

## API

```noxy
use github_com.noxylang.noxy_pty.noxy_pty as pty

let id: int = pty.open("/bin/sh", "/tmp", 80, 24)
pty.write(id, base64_encode("echo ola\n"))
print(to_str(base64_decode(pty.read(id, 500))))
pty.close(id)
```

| Função | Efeito |
|---|---|
| `open(cmd, cwd, cols, rows) -> int` | Abre `cmd` numa pty (`TERM=xterm-256color`) e devolve o id. Vários por processo. No Windows `cmd` é o caminho do shell (`%COMSPEC%`, `pwsh.exe`...) |
| `read(id, timeout_ms) -> string` | Espera até `timeout_ms` por saída; devolve base64, `""` no timeout. Quando o processo terminou e não há mais saída, falha com `pty N exited` |
| `write(id, data_b64)` | Escreve bytes (base64) na entrada |
| `resize(id, cols, rows)` | Avisa o tamanho novo (SIGWINCH; `ResizePseudoConsole` no Windows) |
| `close(id)` | SIGHUP no processo e fecha; no Windows termina o processo e fecha o pseudoconsole, o que encerra também o que ele abriu. Idempotente |

Os dados cruzam em base64 porque a saída de um terminal não é UTF-8
garantido; `base64_encode` e `base64_decode` são builtins do Noxy. Toda falha
é um erro de runtime `extension 'pty' failed: <motivo>`, capturável com
`call_result`. Os terminais morrem com o processo da extensão (quando o
programa Noxy termina). No Windows a saída do pseudoconsole vem em UTF-8 com
sequências VT, que o xterm.js entende direto; Enter é `\r`.

## Desenvolvimento

    go test ./... -race
    sh release/build.sh pty && mkdir -p bin && cp dist/noxy-plugin-pty-linux-amd64 bin/

No Windows, `go build -o bin/noxy-plugin-pty-windows-amd64.exe .`; o
`go test` ali roda um `cmd.exe` de verdade num pseudoconsole. O estado fica
no pacote `ptys/`, testado com um processo falso e com um shell real; o
processo de cada plataforma está em `starter_unix.go` e `starter_windows.go`.
Para usar um checkout num projeto, linke-o em
`<projeto>/noxy_libs/github_com/noxylang/noxy_pty` (no Windows, uma junção:
`mklink /J`); então
`noxy noxy_libs/github_com/noxylang/noxy_pty/examples/smoke.nx` imprime `ok`.

A CI (`.github/workflows/ci.yml`) roda os testes no Ubuntu e no Windows.
Release: push de uma tag `vX.Y.Z`. Tudo é Go puro (`creack/pty` e
`x/sys/windows`), então o workflow compila todas as plataformas num runner
só, sem cgo.
