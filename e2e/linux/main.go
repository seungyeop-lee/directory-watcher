package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/worktree"
)

func main() { os.Exit(run()) }

func run() (code int) {
	deadline := flag.Duration("deadline", 10*time.Minute, "container execution deadline")
	image := flag.String("image", "golang:1.23-bookworm", "Linux Go image")
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	root, err = worktree.Root(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	logs, err := os.MkdirTemp("", "dw-e2e-linux-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Linux diagnostics:", logs)
	logFile, err := os.Create(filepath.Join(logs, "runner.log"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer logFile.Close()
	output := io.MultiWriter(os.Stdout, logFile)
	ctx, cancel := context.WithTimeout(context.Background(), *deadline)
	defer cancel()
	if err := command(ctx, output, "docker", "image", "inspect", "--format", "{{.Id}}", *image); err != nil {
		if err := command(ctx, output, "docker", "pull", *image); err != nil {
			return 1
		}
	}
	containerName := filepath.Base(logs)
	args := []string{"create", "--name", containerName, "--init", "--label", "directory-watcher.e2e=true", "--workdir", "/src", "--env", "DW_E2E_KEEP=1"}
	if fault := os.Getenv("DW_E2E_FAULT"); fault != "" {
		args = append(args, "--env", "DW_E2E_FAULT="+fault)
	}
	args = append(args, *image, "go", "test", "-count=1", "-v")
	args = append(args, flag.Args()...)
	args = append(args, "./e2e/scenarios")
	created, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		fmt.Fprintln(output, "create:", err, string(created))
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = command(cleanupCtx, output, "docker", "rm", "--force", containerName)
		return 1
	}
	id := strings.TrimSpace(string(created))
	fmt.Fprintln(output, "container:", id)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := command(cleanupCtx, output, "docker", "stop", "--timeout", "3", id); err != nil {
			fmt.Fprintln(output, "container stop:", err)
			code = 1
		}
		if err := command(cleanupCtx, output, "docker", "cp", id+":/tmp/.", logs); err != nil {
			fmt.Fprintln(output, "diagnostic copy:", err)
			code = 1
		}
		if err := command(cleanupCtx, output, "docker", "rm", "--force", id); err != nil {
			fmt.Fprintln(output, "container removal:", err)
			code = 1
		}
	}()
	files, err := worktree.Files(root)
	if err != nil {
		fmt.Fprintln(output, err)
		return 1
	}
	reader, writer := io.Pipe()
	snapshotDone := make(chan error, 1)
	go func() {
		err := worktree.Archive(writer, root, files)
		_ = writer.CloseWithError(err)
		snapshotDone <- err
	}()
	copyCmd := exec.CommandContext(ctx, "docker", "cp", "-", id+":/")
	copyCmd.Stdin = reader
	copyCmd.Stdout, copyCmd.Stderr = output, output
	err = copyCmd.Run()
	_ = reader.Close()
	snapshotErr := <-snapshotDone
	if err != nil || snapshotErr != nil {
		fmt.Fprintln(output, "snapshot:", err, snapshotErr)
		return 1
	}
	if err := command(ctx, output, "docker", "start", "--attach", id); err != nil {
		fmt.Fprintln(output, "Linux E2E:", err, "deadline:", ctx.Err())
		return 1
	}
	return 0
}

func command(ctx context.Context, out io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}
