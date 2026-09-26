# noxy_pty

Terminais de verdade (pty) para programas [Noxy](https://github.com/estevaofon/noxy),
como extensão por processo sobre [creack/pty](https://github.com/creack/pty).
Feita para o terminal do [Noxy Editor](https://github.com/estevaofon/Noxy-Editor);
serve para qualquer programa Noxy que precise rodar um shell ou um programa
interativo e ler e escrever nele.

## Instalação

    noxy --get github.com/estevaofon/noxy_pty

Binários para Linux (amd64, arm64), macOS (Intel, Apple Silicon) e Windows.
Requer Noxy 0.25.0 ou mais novo. No Windows o binário existe para o `use`
compilar, e `open` responde "terminal indisponivel no Windows nesta versao".

## API

```noxy
use github_com.estevaofon.noxy_pty.noxy_pty as pty

let id: int = pty.open("/bin/sh", "/tmp", 80, 24)
pty.write(id, base64_encode("echo ola\n"))
print(to_str(base64_decode(pty.read(id, 500))))
pty.close(id)
```

| Função | Efeito |
|---|---|
| `open(cmd, cwd, cols, rows) -> int` | Abre `cmd` numa pty (`TERM=xterm-256color`) e devolve o id. Vários por processo |
| `read(id, timeout_ms) -> string` | Espera até `timeout_ms` por saída; devolve base64, `""` no timeout. Quando o processo terminou e não há mais saída, falha com `pty N exited` |
| `write(id, data_b64)` | Escreve bytes (base64) na entrada |
| `resize(id, cols, rows)` | Avisa o tamanho novo (SIGWINCH) |
| `close(id)` | SIGHUP no processo e fecha; idempotente |

Os dados cruzam em base64 porque a saída de um terminal não é UTF-8
garantido; `base64_encode` e `base64_decode` são builtins do Noxy. Toda falha
é um erro de runtime `extension 'pty' failed: <motivo>`, capturável com
`call_result`. Os terminais morrem com o processo da extensão (quando o
programa Noxy termina).

## Desenvolvimento

    go test ./... -race
    sh release/build.sh pty && mkdir -p bin && cp dist/noxy-plugin-pty-linux-amd64 bin/

O estado fica no pacote `ptys/`, testado com um processo falso e com um shell
real. Para usar um checkout num projeto, linke-o em
`<projeto>/noxy_libs/github_com/estevaofon/noxy_pty`; então
`noxy noxy_libs/github_com/estevaofon/noxy_pty/examples/smoke.nx` imprime `ok`.

Release: push de uma tag `vX.Y.Z`. Como `creack/pty` é Go puro, o workflow
compila todas as plataformas num runner só, sem cgo.
