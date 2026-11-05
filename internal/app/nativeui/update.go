package nativeui

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VibePiik updates itself from its GitHub releases. Release-notes rules (the
// same as PIIK TV):
//   - <!-- importance: important --> in the notes makes the update important
//     (the line is invisible on GitHub); without it the update is optional;
//   - the changelog shown is the summary line in the App's language (**EN**,
//     **IT**, **中文**, after the --- line) followed by the main English text,
//     without the "Install" section and without code blocks.
//
// Installing downloads VibePiik-windows-amd64.zip, checks its SHA-256 with the
// digest GitHub publishes, unpacks it next to the App, moves the running files
// aside (*.old: Windows lets a running program be renamed), puts the new ones
// in their place and starts the new App, which waits for this one to quit and
// then removes the old files.

var importantPattern = regexp.MustCompile(`(?i)<!--\s*importance\s*:\s*important\s*-->`)

func releaseImportant(body string) bool { return importantPattern.MatchString(body) }

var (
	codeBlocks   = regexp.MustCompile("(?s)```.*?```")
	htmlComments = regexp.MustCompile(`(?s)<!--.*?-->`)
	ruleLine     = regexp.MustCompile(`(?m)^-{3,}\s*$`)
	installPart  = regexp.MustCompile(`(?ms)^###\s*(Install|Installazione|安装)\b.*?(?:^###|\z)`)
	mdLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	mdHeading    = regexp.MustCompile(`^#{1,6}\s*`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
)

// releaseChangelog turns the release notes into plain text for lang.
func releaseChangelog(body, lang string) string {
	text := strings.ReplaceAll(body, "\r\n", "\n")
	text = codeBlocks.ReplaceAllString(text, "")
	text = htmlComments.ReplaceAllString(text, "")
	main, tail := text, ""
	if at := ruleLine.FindStringIndex(text); at != nil {
		main, tail = text[:at[0]], text[at[1]:]
	}
	marker := map[string]string{"it": "**IT**", "zh": "**中文**"}[lang]
	if marker == "" {
		marker = "**EN**"
	}
	summary := ""
	for _, line := range strings.Split(tail, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, marker) {
			summary = strings.TrimLeft(strings.TrimPrefix(line, marker), " —-:")
			break
		}
	}
	// The install section ends at the next heading, which must stay.
	main = installPart.ReplaceAllStringFunc(main, func(section string) string {
		if strings.HasSuffix(section, "###") {
			return "###"
		}
		return ""
	})
	var lines []string
	for _, line := range strings.Split(main, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue // tables
		}
		line = plainMarkdown(line)
		if strings.HasPrefix(line, "- ") {
			line = "• " + line[2:]
		}
		lines = append(lines, line)
	}
	plain := strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
	parts := []string{}
	if summary = plainMarkdown(summary); summary != "" {
		parts = append(parts, summary)
	}
	if plain != "" {
		parts = append(parts, plain)
	}
	return strings.Join(parts, "\n\n")
}

func plainMarkdown(line string) string {
	line = mdHeading.ReplaceAllString(line, "")
	line = mdLink.ReplaceAllString(line, "$1")
	return strings.NewReplacer("**", "", "`", "").Replace(line)
}

// updater is the one update in progress (UI goroutine reads, worker writes).
var updater struct {
	sync.Mutex
	state    string // "", "download", "apply", "failed"
	progress int    // 0..100
	err      string
	quit     func() // closes the App once the new one is started
}

func updateState() (state string, progress int, err string) {
	updater.Lock()
	defer updater.Unlock()
	return updater.state, updater.progress, updater.err
}

func setUpdateState(state string, progress int, err string) {
	updater.Lock()
	updater.state, updater.progress, updater.err = state, progress, err
	updater.Unlock()
}

// installUpdate downloads, checks and applies notice, then restarts the App.
// It runs on its own goroutine; invalidate redraws the window.
func installUpdate(notice *releaseNotice, invalidate func()) {
	fail := func(err error) {
		setUpdateState("failed", 0, err.Error())
		invalidate()
	}
	if notice == nil || !notice.Package {
		fail(errors.New("no package"))
		return
	}
	setUpdateState("download", 0, "")
	invalidate()
	exe, err := os.Executable()
	if err != nil {
		fail(err)
		return
	}
	dir := filepath.Dir(exe)
	archive := filepath.Join(os.TempDir(), "VibePiik-update-"+notice.Version+".zip")
	if err := downloadPackage(notice.URL, archive, notice.Digest, func(p int) {
		setUpdateState("download", p, "")
		invalidate()
	}); err != nil {
		fail(err)
		return
	}
	setUpdateState("apply", 100, "")
	invalidate()
	staged := filepath.Join(dir, ".update")
	_ = os.RemoveAll(staged)
	if err := unpack(archive, staged); err != nil {
		fail(err)
		return
	}
	_ = os.Remove(archive)
	if _, err := os.Stat(filepath.Join(staged, filepath.Base(exe))); err != nil {
		fail(errors.New("the package has no " + filepath.Base(exe)))
		return
	}
	if err := swapIn(staged, dir); err != nil {
		fail(err)
		return
	}
	_ = os.RemoveAll(staged)
	if err := restartApp(exe); err != nil {
		fail(err)
		return
	}
	updater.Lock()
	quit := updater.quit
	updater.Unlock()
	if quit != nil {
		quit()
	}
}

var downloadClient = &http.Client{Timeout: 10 * time.Minute}

func downloadPackage(url, dest, digest string, progress func(int)) error {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "VibePiik")
	response, err := downloadClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", response.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	hash := sha256.New()
	total, done, last := response.ContentLength, int64(0), -1
	buf := make([]byte, 256*1024)
	for {
		n, readErr := response.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				file.Close()
				return err
			}
			hash.Write(buf[:n])
			done += int64(n)
			if total > 0 {
				if p := int(done * 100 / total); p != last {
					last = p
					progress(p)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			file.Close()
			return readErr
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	if digest != "" && hex.EncodeToString(hash.Sum(nil)) != digest {
		_ = os.Remove(dest)
		return errors.New("the download is damaged (SHA-256 mismatch)")
	}
	return nil
}

// unpack extracts archive into dir, refusing paths that leave it.
func unpack(archive, dir string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	root, _ := filepath.Abs(dir)
	for _, entry := range reader.File {
		target := filepath.Join(root, filepath.FromSlash(entry.Name))
		if !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return errors.New("unsafe path in the package: " + entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		if err := out.Close(); copyErr == nil {
			copyErr = err
		}
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// swapIn moves every staged file to dir; the files it replaces become *.old
// (a running program can be renamed on Windows, not overwritten). On any
// error the files already moved are put back.
func swapIn(staged, dir string) error {
	type moved struct{ target, old string }
	var done []moved
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			_ = os.Remove(done[i].target)
			if done[i].old != "" {
				_ = os.Rename(done[i].old, done[i].target)
			}
		}
	}
	err := filepath.Walk(staged, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(staged, path)
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		step := moved{target: target}
		if _, statErr := os.Stat(target); statErr == nil {
			step.old = target + ".old"
			_ = os.Remove(step.old)
			if err := os.Rename(target, step.old); err != nil {
				return fmt.Errorf("cannot replace %s: %w", rel, err)
			}
		}
		if err := os.Rename(path, target); err != nil {
			if step.old != "" {
				_ = os.Rename(step.old, target)
			}
			return fmt.Errorf("cannot install %s: %w", rel, err)
		}
		done = append(done, step)
		return nil
	})
	if err != nil {
		rollback()
		return err
	}
	// The files set aside, for FinishUpdate to remove (and nothing else).
	var olds []string
	for _, step := range done {
		if step.old != "" {
			if rel, relErr := filepath.Rel(dir, step.old); relErr == nil {
				olds = append(olds, rel)
			}
		}
	}
	return os.WriteFile(filepath.Join(dir, oldListName), []byte(strings.Join(olds, "\n")), 0o644)
}

// oldListName lists the *.old files an update set aside, next to the App.
const oldListName = ".update-old.txt"

// restartApp starts the new App with the same arguments; it waits for this
// process (VIBEPIIK_UPDATED_FROM) before it opens its ports.
func restartApp(exe string) error {
	command := exec.Command(exe, os.Args[1:]...)
	command.Dir = filepath.Dir(exe)
	command.Env = append(os.Environ(), "VIBEPIIK_UPDATED_FROM="+strconv.Itoa(os.Getpid()), "VIBEPIIK_RESTARTED=1")
	detachConsole(command)
	return command.Start()
}

// FinishUpdate runs first in an App started by an update: it waits for the
// previous App to quit (so its ports are free) and removes the old files.
// In any other start it only removes old files left behind.
func FinishUpdate() {
	if pid, err := strconv.Atoi(os.Getenv("VIBEPIIK_UPDATED_FROM")); err == nil && pid > 0 {
		waitForExit(pid, 20*time.Second)
		os.Unsetenv("VIBEPIIK_UPDATED_FROM")
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exe)
	list := filepath.Join(dir, oldListName)
	data, err := os.ReadFile(list)
	if err != nil {
		return
	}
	left := []string{}
	for _, rel := range strings.Split(string(data), "\n") {
		rel = strings.TrimSpace(rel)
		path := filepath.Join(dir, rel)
		// Only files this App set aside: inside its folder, ending in .old.
		if rel == "" || !strings.HasSuffix(rel, ".old") || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			left = append(left, rel) // still in use: next start
		}
	}
	if len(left) == 0 {
		_ = os.Remove(list)
	} else {
		_ = os.WriteFile(list, []byte(strings.Join(left, "\n")), 0o644)
	}
	_ = os.RemoveAll(filepath.Join(dir, ".update"))
}

// SetUpdateQuit is how an installed update closes the running App.
func SetUpdateQuit(quit func()) {
	updater.Lock()
	updater.quit = quit
	updater.Unlock()
}

func init() {
	if feed := os.Getenv("VIBEPIIK_UPDATE_FEED"); strings.HasPrefix(feed, "http://127.0.0.1:") {
		// Automated tests only: a fake release served from this PC.
		releaseAPI, releaseTestFeed = feed, true
	}
}
