package nativeui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleNotes = "First release.\n\n### Highlights\n- **Native window** for the [room](https://x).\n\n" +
	"### Install\nDownload `VibePiik-windows-amd64.zip`.\n\n| a | b |\n| --- | --- |\n\n**SHA-256**\n```\nabc  file.zip\n```\n\n" +
	"<!-- importance: important -->\n\n---\n**EN** — English summary.\n**IT** — Riassunto italiano.\n**中文** — 中文摘要。\n"

func TestReleaseChangelog(t *testing.T) {
	if !releaseImportant(sampleNotes) || releaseImportant("plain notes") {
		t.Fatal("importance marker")
	}
	it := releaseChangelog(sampleNotes, "it")
	for _, want := range []string{"Riassunto italiano.", "Highlights", "• Native window for the room."} {
		if !strings.Contains(it, want) {
			t.Fatalf("missing %q in:\n%s", want, it)
		}
	}
	for _, unwanted := range []string{"Download", "abc", "importance", "| a", "English summary", "**", "](https"} {
		if strings.Contains(it, unwanted) {
			t.Fatalf("unexpected %q in:\n%s", unwanted, it)
		}
	}
	if zh := releaseChangelog(sampleNotes, "zh"); !strings.HasPrefix(zh, "中文摘要。") {
		t.Fatalf("zh summary first:\n%s", zh)
	}
}

func TestSwapIn(t *testing.T) {
	dir, staged := t.TempDir(), t.TempDir()
	write := func(path, text string) {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "piik-app.exe"), "old app")
	write(filepath.Join(dir, "runtime", "native", "piik-capture.exe"), "old capture")
	write(filepath.Join(dir, "user-notes.txt.old"), "not ours")
	write(filepath.Join(staged, "piik-app.exe"), "new app")
	write(filepath.Join(staged, "runtime", "native", "piik-capture.exe"), "new capture")
	write(filepath.Join(staged, "README-VibePiik.txt"), "new readme")
	if err := swapIn(staged, dir); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"piik-app.exe": "new app", "runtime/native/piik-capture.exe": "new capture", "README-VibePiik.txt": "new readme",
		"piik-app.exe.old": "old app", "runtime/native/piik-capture.exe.old": "old capture",
	} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q %v", path, data, err)
		}
	}
	list, _ := os.ReadFile(filepath.Join(dir, oldListName))
	if strings.Contains(string(list), "user-notes") || !strings.Contains(string(list), "piik-app.exe.old") {
		t.Fatalf("old list: %q", list)
	}
}
