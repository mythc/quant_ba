package strategy

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/colinmyth/quant_ba/internal/types"
)

// Loader manages strategy plugin subprocesses.
// It starts plugin binaries, communicates with them via JSON-RPC over
// stdin/stdout, and tracks their lifecycle.
type Loader struct {
	mu      sync.RWMutex
	plugins map[string]*LoadedStrategy
}

// LoadedStrategy holds the runtime state of a single strategy plugin process.
type LoadedStrategy struct {
	Meta   types.StrategyMeta
	Client *PluginClient
	Cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// NewLoader creates a new Loader.
func NewLoader() *Loader {
	return &Loader{
		plugins: make(map[string]*LoadedStrategy),
	}
}

// Load starts a strategy plugin process and returns its metadata.
func (l *Loader) Load(path string) (*types.StrategyMeta, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin: %w", err)
	}

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(stdout)
	client := NewPluginClient(enc, dec)

	var meta MetaResult
	if err := client.Call("meta", nil, &meta); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("get meta: %w", err)
	}

	ls := &LoadedStrategy{
		Meta: types.StrategyMeta{
			ID:      meta.ID,
			Name:    meta.Name,
			Version: meta.Version,
			Path:    path,
		},
		Client: client,
		Cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}

	l.plugins[meta.ID] = ls
	return &ls.Meta, nil
}

// Unload stops a plugin process and removes it from the loader.
func (l *Loader) Unload(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	ls, ok := l.plugins[id]
	if !ok {
		return fmt.Errorf("plugin %s not loaded", id)
	}

	ls.Client.Call("stop", nil, nil)
	ls.stdin.Close()
	ls.Cmd.Wait()
	delete(l.plugins, id)
	return nil
}

// Get returns the RPC client for a loaded strategy.
func (l *Loader) Get(id string) (*LoadedStrategy, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	ls, ok := l.plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin %s not loaded", id)
	}
	return ls, nil
}

// List returns metadata for all loaded plugins.
func (l *Loader) List() []types.StrategyMeta {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var metas []types.StrategyMeta
	for _, ls := range l.plugins {
		metas = append(metas, ls.Meta)
	}
	return metas
}

// Close unloads all plugins.
func (l *Loader) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, ls := range l.plugins {
		ls.Client.Call("stop", nil, nil)
		ls.stdin.Close()
		ls.Cmd.Wait()
		delete(l.plugins, id)
	}
}
