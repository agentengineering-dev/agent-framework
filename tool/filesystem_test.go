package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, input string) (string, error) {
	t.Helper()
	return ReadFileImpl(json.RawMessage(input))
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadFileWholeFileIsNumbered(t *testing.T) {
	path := writeTemp(t, "a.txt", "one\ntwo\nthree\n")

	got, err := readFile(t, fmt.Sprintf(`{"path":%q,"offset":0,"limit":0}`, path))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\tone\n2\ttwo\n3\tthree\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadFileWindow(t *testing.T) {
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	path := writeTemp(t, "b.txt", strings.Join(lines, "\n")+"\n")

	got, err := readFile(t, fmt.Sprintf(`{"path":%q,"offset":4,"limit":3}`, path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "4\tline 4\n5\tline 5\n6\tline 6\n") {
		t.Fatalf("unexpected window: %q", got)
	}
	if !strings.Contains(got, "continue at offset 7") {
		t.Fatalf("missing continuation hint: %q", got)
	}
}

func TestReadFileLastLineWithoutNewline(t *testing.T) {
	path := writeTemp(t, "c.txt", "one\ntwo")

	got, err := readFile(t, fmt.Sprintf(`{"path":%q,"offset":2,"limit":0}`, path))
	if err != nil {
		t.Fatal(err)
	}
	if got != "2\ttwo\n" {
		t.Fatalf("got %q", got)
	}
}

func TestReadFileOffsetPastEnd(t *testing.T) {
	path := writeTemp(t, "d.txt", "one\ntwo\n")

	if _, err := readFile(t, fmt.Sprintf(`{"path":%q,"offset":9,"limit":0}`, path)); err == nil {
		t.Fatal("expected an error for an offset past the end of the file")
	}
}

func TestReadFileRejects(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("SECRET=1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"dotenv":          fmt.Sprintf(`{"path":%q}`, env),
		"directory":       fmt.Sprintf(`{"path":%q}`, dir),
		"negative offset": fmt.Sprintf(`{"path":%q,"offset":-1}`, env+".missing"),
	}
	for name, input := range cases {
		if _, err := readFile(t, input); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func TestReadFileEmptyFile(t *testing.T) {
	path := writeTemp(t, "e.txt", "")

	got, err := readFile(t, fmt.Sprintf(`{"path":%q}`, path))
	if err != nil {
		t.Fatal(err)
	}
	if got != "(file is empty)" {
		t.Fatalf("got %q", got)
	}
}

func TestReadFileTruncatesLongLine(t *testing.T) {
	path := writeTemp(t, "f.txt", strings.Repeat("x", maxReadLineWidth+50)+"\n")

	got, err := readFile(t, fmt.Sprintf(`{"path":%q}`, path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "line truncated") {
		t.Fatalf("expected truncation: %q", got[:80])
	}
}

func TestListFilesShowsHumanReadableSizes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), make([]byte, 4300), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := ListFileImpl(json.RawMessage(fmt.Sprintf(`{"directory":%q}`, dir)))
	if err != nil {
		t.Fatal(err)
	}
	want := "big.bin (4.2 KB)\nsmall.txt (5 B)\nsub/"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{4300, "4.2 KB"},
		{10 * 1024, "10 KB"},
		{524_800, "512 KB"},
		{1024 * 1024, "1.0 MB"},
		{3_221_225_472, "3.0 GB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
