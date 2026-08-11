package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mathif92/olympus/prometheus/pkg/database"
)

// DockerExecutorConfig tunes the Docker-backed executor.
type DockerExecutorConfig struct {
	ImagePrefix  string
	BuildTimeout time.Duration
	DockerBinary string
}

// DockerExecutor builds a per-runtime Docker image for each function version
// (cached by version id) and runs the handler in a constrained container:
// the event is streamed on stdin, the handler's JSON result is read from
// stdout, and timeout / memory / CPU limits are enforced by the container.
type DockerExecutor struct {
	cfg     DockerExecutorConfig
	buildMu sync.Mutex
}

// NewDockerExecutor creates a DockerExecutor with sensible defaults.
func NewDockerExecutor(cfg DockerExecutorConfig) *DockerExecutor {
	if cfg.ImagePrefix == "" {
		cfg.ImagePrefix = "olympus/prometheus-fn"
	}
	if cfg.BuildTimeout <= 0 {
		cfg.BuildTimeout = 2 * time.Minute
	}
	if cfg.DockerBinary == "" {
		cfg.DockerBinary = "docker"
	}
	return &DockerExecutor{cfg: cfg}
}

// Healthy reports whether the Docker daemon is reachable.
func (d *DockerExecutor) Healthy(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, d.cfg.DockerBinary, "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker daemon unreachable: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Invoke builds (if needed) and runs the function version, enforcing the
// function's timeout, memory and CPU limits.
func (d *DockerExecutor) Invoke(ctx context.Context, fn database.Function, v database.FunctionVersion, event json.RawMessage) (*InvocationResult, error) {
	rt, ok := GetRuntime(fn.Runtime)
	if !ok {
		return nil, fmt.Errorf("unknown runtime %q", fn.Runtime)
	}
	if err := d.ensureBuilt(ctx, v, rt); err != nil {
		return nil, err
	}

	timeout := time.Duration(fn.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	mem := fn.MemoryMB
	if mem <= 0 {
		mem = 128
	}
	cpus := fn.CPUs
	if cpus <= 0 {
		cpus = 0.5
	}

	name := "prom-fn-" + v.ID
	args := []string{
		"run", "--rm", "-i", "--network", "none",
		"--name", name,
		"-m", strconv.Itoa(mem) + "m",
		"--memory-swap", strconv.Itoa(mem) + "m",
		"--cpus", strconv.FormatFloat(cpus, 'f', -1, 64),
	}
	for k, val := range fn.EnvVars {
		args = append(args, "-e", k+"="+val)
	}
	args = append(args, d.imageTag(v.ID))

	// Allow a short grace period beyond the function timeout so a timed-out
	// container can be killed and reaped cleanly.
	runCtx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()

	started := time.Now()
	cmd := exec.Command(d.cfg.DockerBinary, args...)
	cmd.Stdin = strings.NewReader(string(event))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				exitCode = ee.ExitCode()
			}
		}
		status := StatusSuccess
		if err != nil {
			status = StatusError
			if exitCode == 137 {
				status = StatusOOM
			}
		}
		return &InvocationResult{
			Status:     status,
			Response:   out.String(),
			Error:      errBuf.String(),
			ExitCode:   exitCode,
			DurationMS: time.Since(started).Milliseconds(),
		}, nil

	case <-runCtx.Done():
		// Hard timeout: force-kill the container (best effort), then reap.
		_ = exec.Command(d.cfg.DockerBinary, "kill", name).Run()
		<-done
		return &InvocationResult{
			Status:     StatusTimeout,
			Error:      fmt.Sprintf("function exceeded timeout of %s", timeout),
			ExitCode:   124,
			DurationMS: timeout.Milliseconds(),
		}, nil
	}
}

func (d *DockerExecutor) imageTag(versionID string) string {
	return d.cfg.ImagePrefix + ":" + versionID
}

// ensureBuilt builds the version's image once (tagged by version id); later
// invocations reuse the cached image.
func (d *DockerExecutor) ensureBuilt(ctx context.Context, v database.FunctionVersion, rt Runtime) error {
	tag := d.imageTag(v.ID)
	if d.hasImage(ctx, tag) {
		return nil
	}
	d.buildMu.Lock()
	defer d.buildMu.Unlock()
	if d.hasImage(ctx, tag) {
		return nil
	}
	return d.buildImage(ctx, v, rt, tag)
}

func (d *DockerExecutor) hasImage(ctx context.Context, tag string) bool {
	return exec.CommandContext(ctx, d.cfg.DockerBinary, "image", "inspect", tag).Run() == nil
}

// buildImage materialises a build context: user code extracted from the zip
// plus the runtime's Dockerfile and launcher scaffolding, then docker build.
func (d *DockerExecutor) buildImage(ctx context.Context, v database.FunctionVersion, rt Runtime, tag string) error {
	work, err := os.MkdirTemp("", "prometheus-fn-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	if err := extractZip(v.Code, work); err != nil {
		return fmt.Errorf("extract code: %w", err)
	}
	rtFS, err := fs.Sub(runtimesFS, filepath.Join("runtimes", rt.ID))
	if err != nil {
		return err
	}
	if err := copyFS(work, rtFS, "."); err != nil {
		return fmt.Errorf("materialise runtime scaffolding: %w", err)
	}
	if rt.ID == goRuntimeID {
		// The Go runtime needs a module file to build; it is generated rather
		// than embedded (go:embed refuses directories containing a go.mod).
		if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte(goModuleFile), 0o644); err != nil {
			return fmt.Errorf("write go.mod: %w", err)
		}
		// The embedded launcher is shipped as a template (a .go file inside the
		// package tree would be compiled); materialise it as main.go.
		tmpl := filepath.Join(work, "main.go.tmpl")
		if err := os.Rename(tmpl, filepath.Join(work, "main.go")); err != nil {
			return fmt.Errorf("materialise main.go: %w", err)
		}
	}

	buildCtx, cancel := context.WithTimeout(ctx, d.cfg.BuildTimeout)
	defer cancel()
	cmd := exec.CommandContext(buildCtx, d.cfg.DockerBinary, "build", "-t", tag, work)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build %s: %v: %s", tag, err, truncate(string(out), 2000))
	}
	return nil
}

// copyFS copies every file under fsys (relative to dir) into dst, preserving
// directory structure.
func copyFS(dst string, fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		rel := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if err := copyFS(dst, fsys, rel); err != nil {
				return err
			}
			continue
		}
		src, err := fsys.Open(rel)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "… (truncated)"
}

const goRuntimeID = "go1.25"

// goModuleFile bootstraps the Go runtime build context with its own module.
const goModuleFile = `module olympus/handler

go 1.25
`
