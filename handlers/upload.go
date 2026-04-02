package handlers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/github/copilot-sdk/go"
)

// maxFileSize é o tamanho máximo permitido para upload (5MB).
const maxFileSize = 5 << 20

// uploadBaseDir é o diretório base para arquivos temporários de upload.
// Isolado do /tmp geral para evitar conflitos com o Copilot CLI.
const uploadBaseDir = "/tmp/copilot-uploads"

// allowedExtensions define quais extensões de arquivo são aceitas.
var allowedExtensions = map[string]bool{
	// Imagens
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".svg": true,
	// Texto
	".txt": true, ".md": true, ".json": true, ".yaml": true,
	".yml": true, ".csv": true, ".xml": true, ".html": true,
	// Código
	".go": true, ".py": true, ".js": true, ".ts": true,
	".java": true, ".c": true, ".cpp": true, ".h": true,
	".rs": true, ".rb": true, ".php": true, ".sh": true,
	".sql": true, ".css": true, ".scss": true, ".jsx": true,
	".tsx": true, ".vue": true, ".swift": true, ".kt": true,
	// Documentos
	".pdf": true,
	// Dados
	".log": true, ".env": true, ".toml": true, ".ini": true,
}

// validateFileExtension verifica se a extensão do arquivo é permitida.
func validateFileExtension(filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return fmt.Errorf("arquivo sem extensão não é permitido")
	}
	if !allowedExtensions[ext] {
		return fmt.Errorf("extensão %q não é permitida", ext)
	}
	return nil
}

// saveTempFile salva o conteúdo do reader em um arquivo temporário.
// Retorna o caminho do arquivo e uma função cleanup para removê-lo.
func saveTempFile(r io.Reader, filename string) (tmpPath string, cleanup func(), err error) {
	// Garante que o diretório base existe (necessário fora do Docker).
	if err := os.MkdirAll(uploadBaseDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("falha ao criar diretório de uploads: %w", err)
	}

	dir, err := os.MkdirTemp(uploadBaseDir, "copilot-upload-*")
	if err != nil {
		return "", nil, fmt.Errorf("falha ao criar diretório temporário: %w", err)
	}

	// Usa apenas o nome base para evitar path traversal.
	safeName := filepath.Base(filename)
	fpath := filepath.Join(dir, safeName)

	f, err := os.Create(fpath)
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("falha ao criar arquivo temporário: %w", err)
	}

	// Limita a leitura ao tamanho máximo + 1 byte para detectar excesso.
	limited := io.LimitReader(r, maxFileSize+1)
	n, err := io.Copy(f, limited)
	f.Close()

	if err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("falha ao gravar arquivo temporário: %w", err)
	}
	if n > maxFileSize {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("arquivo excede o limite de %dMB", maxFileSize/(1<<20))
	}

	cleanup = func() { os.RemoveAll(dir) }
	return fpath, cleanup, nil
}

// buildFileAttachment constrói um sdk.Attachment do tipo "file" a partir de um caminho local.
func buildFileAttachment(path, displayName string) sdk.Attachment {
	return sdk.Attachment{
		Type:        sdk.File,
		Path:        &path,
		DisplayName: &displayName,
	}
}
