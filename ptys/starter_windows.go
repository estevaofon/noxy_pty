//go:build windows

package ptys

import "errors"

// Start no Windows ainda nao existe: o binario e publicado para o `use`
// compilar, e abrir um terminal responde com esta mensagem.
func Start(cmd, cwd string, cols, rows int) (Proc, error) {
	return nil, errors.New("terminal indisponivel no Windows nesta versao")
}
