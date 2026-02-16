package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/battlewithbytes/pve-appstore/sdk"
)

// handleDevSDKDocs serves SDK method documentation extracted from the embedded
// Python SDK source. The result is computed once and cached for the lifetime
// of the server.
func (s *Server) handleDevSDKDocs(w http.ResponseWriter, r *http.Request) {
	s.sdkDocsOnce.Do(func() {
		docs, err := extractSDKDocs()
		if err != nil {
			log.Printf("[dev] warning: could not extract SDK docs: %v", err)
			s.sdkDocsJSON = []byte("[]")
			return
		}
		s.sdkDocsJSON = docs
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(s.sdkDocsJSON)
}

// extractSDKDocs runs the embedded extract_docs.py against the embedded SDK
// source files and returns the JSON output.
func extractSDKDocs() ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "sdk-docs-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Extract all Python SDK files to the temp directory
	entries, err := fs.ReadDir(sdk.PythonFS, "python/appstore")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(sdk.PythonFS, "python/appstore/"+entry.Name())
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(tmpDir, entry.Name()), data, 0644); err != nil {
			return nil, err
		}
	}

	// Run the extractor
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", filepath.Join(tmpDir, "extract_docs.py"))
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Validate that the output is valid JSON
	var docs []json.RawMessage
	if err := json.Unmarshal(output, &docs); err != nil {
		return nil, err
	}

	return output, nil
}
