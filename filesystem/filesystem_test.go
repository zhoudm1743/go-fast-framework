package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) (*localDriver, string) {
	t.Helper()
	dir := t.TempDir()
	d, err := newLocalDriver(dir, "/storage")
	if err != nil {
		t.Fatal(err)
	}
	return d, dir
}

func TestPutAndGet(t *testing.T) {
	d, _ := setup(t)
	if err := d.Put("hello.txt", "world"); err != nil {
		t.Fatal(err)
	}
	content, err := d.Get("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "world" {
		t.Fatalf("expected 'world', got %q", content)
	}
}

func TestGetBytes(t *testing.T) {
	d, _ := setup(t)
	_ = d.Put("data.bin", "binary")
	b, err := d.GetBytes("data.bin")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "binary" {
		t.Fatalf("expected 'binary', got %q", string(b))
	}
}

func TestExists(t *testing.T) {
	d, _ := setup(t)
	if d.Exists("nope.txt") {
		t.Fatal("should not exist")
	}
	_ = d.Put("yes.txt", "ok")
	if !d.Exists("yes.txt") {
		t.Fatal("should exist")
	}
	if !d.Missing("nope.txt") {
		t.Fatal("Missing should return true")
	}
}

func TestDelete(t *testing.T) {
	d, _ := setup(t)
	_ = d.Put("a.txt", "a")
	_ = d.Put("b.txt", "b")
	if err := d.Delete("a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	if d.Exists("a.txt") || d.Exists("b.txt") {
		t.Fatal("files should be deleted")
	}
}

func TestCopy(t *testing.T) {
	d, _ := setup(t)
	_ = d.Put("src.txt", "data")
	if err := d.Copy("src.txt", "dst.txt"); err != nil {
		t.Fatal(err)
	}
	v, _ := d.Get("dst.txt")
	if v != "data" {
		t.Fatalf("expected 'data', got %q", v)
	}
	if !d.Exists("src.txt") {
		t.Fatal("source should still exist after copy")
	}
}

func TestMove(t *testing.T) {
	d, _ := setup(t)
	_ = d.Put("old.txt", "moved")
	if err := d.Move("old.txt", "new.txt"); err != nil {
		t.Fatal(err)
	}
	if d.Exists("old.txt") {
		t.Fatal("old file should not exist after move")
	}
	v, _ := d.Get("new.txt")
	if v != "moved" {
		t.Fatalf("expected 'moved', got %q", v)
	}
}

func TestSize(t *testing.T) {
	d, _ := setup(t)
	_ = d.Put("s.txt", "12345")
	sz, err := d.Size("s.txt")
	if err != nil {
		t.Fatal(err)
	}
	if sz != 5 {
		t.Fatalf("expected 5, got %d", sz)
	}
}

func TestUrl(t *testing.T) {
	d, _ := setup(t)
	url := d.Url("img/avatar.png")
	if url != "/storage/img/avatar.png" {
		t.Fatalf("unexpected url: %s", url)
	}
}

func TestMimeType(t *testing.T) {
	d, _ := setup(t)
	m, _ := d.MimeType("photo.jpg")
	if m != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %s", m)
	}
	m, _ = d.MimeType("unknown.xyz")
	if m != "application/octet-stream" {
		t.Fatalf("expected application/octet-stream, got %s", m)
	}
}

func TestDirectoryOps(t *testing.T) {
	d, _ := setup(t)
	if err := d.MakeDirectory("sub/dir"); err != nil {
		t.Fatal(err)
	}
	_ = d.Put("sub/dir/a.txt", "a")
	_ = d.Put("sub/dir/b.txt", "b")
	_ = d.MakeDirectory("sub/dir/nested")

	files, err := d.Files("sub/dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	dirs, err := d.Directories("sub/dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}

	if err := d.DeleteDirectory("sub"); err != nil {
		t.Fatal(err)
	}
	if d.Exists("sub/dir/a.txt") {
		t.Fatal("should be deleted recursively")
	}
}

func TestAllFilesRecursive(t *testing.T) {
	d, _ := setup(t)
	_ = d.Put("a.txt", "a")
	_ = d.Put("sub/b.txt", "b")
	_ = d.Put("sub/deep/c.txt", "c")

	files, err := d.AllFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(files), files)
	}
}

func TestNestedPut(t *testing.T) {
	d, dir := setup(t)
	// Put 应自动创建中间目录
	if err := d.Put("deep/nested/file.txt", "ok"); err != nil {
		t.Fatal(err)
	}
	fullPath := filepath.Join(dir, "deep", "nested", "file.txt")
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("nested file should exist: %v", err)
	}
}

func TestPath(t *testing.T) {
	d, dir := setup(t)
	p := d.Path("img/a.png")
	expected := filepath.Join(dir, "img", "a.png")
	if p != expected {
		t.Fatalf("expected %s, got %s", expected, p)
	}
}
