package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// initTestRepo membuat repo "origin" dengan satu commit.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
}

func TestGitHelpersBehindAndPull(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git tidak tersedia")
	}
	base := t.TempDir()
	origin := filepath.Join(base, "origin")
	clone := filepath.Join(base, "clone")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, origin)

	// clone (file:// agar protokol lokal diizinkan)
	cmd := exec.Command("git", "clone", "file://"+origin, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v (%s)", err, out)
	}

	// kondisi awal: 0 di belakang
	if err := gitFetch(context.Background(), clone); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if n, err := gitBehind(context.Background(), clone); err != nil || n != 0 {
		t.Fatalf("behind = %d, %v; mau 0", n, err)
	}

	// commit baru di origin → behind 1
	if err := os.WriteFile(filepath.Join(origin, "README.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("git", "-C", origin, "commit", "-am", "v2")
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("commit v2: %v (%s)", err, out)
	}

	if err := gitFetch(context.Background(), clone); err != nil {
		t.Fatal(err)
	}
	n, err := gitBehind(context.Background(), clone)
	if err != nil || n != 1 {
		t.Fatalf("behind = %d, %v; mau 1", n, err)
	}

	// reset --hard origin/main (jalur yang dipakai update) → head sinkron
	if err := gitCmd(context.Background(), clone, "reset", "--hard", "origin/main").Run(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n, _ := gitBehind(context.Background(), clone); n != 0 {
		t.Fatalf("setelah reset behind = %d; mau 0", n)
	}
	b, _ := os.ReadFile(filepath.Join(clone, "README.md"))
	if string(b) != "v2" {
		t.Fatalf("isi = %q; mau v2", b)
	}
}

func TestEnsureMirrorClones(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git tidak tersedia")
	}
	base := t.TempDir()
	origin := filepath.Join(base, "origin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, origin)

	// mirrorDir ditimpa sementara lewat env JR_REPO_DIR
	t.Setenv("JR_REPO_DIR", filepath.Join(base, "mirror"))
	t.Setenv("JR_DATA_DIR", filepath.Join(base, "data"))

	mctx, mcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer mcancel()
	dir, err := ensureMirror(mctx, &updateJob{})
	if err != nil {
		t.Fatalf("ensureMirror: %v", err)
	}
	if !isRepo(dir) || dir != filepath.Join(base, "mirror") {
		t.Fatalf("dir = %s", dir)
	}
	// panggil kedua kali → pakai yang sudah ada
	dir2, err := ensureMirror(context.Background(), &updateJob{})
	if err != nil || dir2 != dir {
		t.Fatalf("mirror kedua = %s, %v", dir2, err)
	}
}

func TestFindLlamastashBin(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "llamastash")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir) // hanya berisi fake — LookPath menemukannya
	if got := findLlamastashBin(); got != fake {
		t.Fatalf("find = %q; mau %q", got, fake)
	}
	// PATH kosong → fallback ke lokasi umum (tidak ada di test → "")
	t.Setenv("PATH", "")
	if got := findLlamastashBin(); got != "" {
		t.Logf("fallback menemukan %q (ok bila memang terpasang)", got)
	}
}

func TestInContainer(t *testing.T) {
	old := containerMarkerPath
	t.Cleanup(func() { containerMarkerPath = old })
	containerMarkerPath = filepath.Join(t.TempDir(), "tidak-ada")
	if inContainer() {
		t.Fatal("marker tidak ada → harus false")
	}
	containerMarkerPath = filepath.Join(t.TempDir(), "ada")
	if err := os.WriteFile(containerMarkerPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !inContainer() {
		t.Fatal("marker ada → harus true")
	}
}

func TestExtractBearerLike(t *testing.T) {
	if got := extractBearerLike([]byte(`{"ok":true,"data":{"bearer_key":"ls-1234567890abcdef"}}`)); got != "ls-1234567890abcdef" {
		t.Fatalf("got = %q", got)
	}
	if got := extractBearerLike([]byte(`bukan json`)); got != "" {
		t.Fatalf("got = %q", got)
	}
	if got := extractBearerLike([]byte(`{"name":"x"}`)); got != "" {
		t.Fatalf("got = %q", got)
	}
}
