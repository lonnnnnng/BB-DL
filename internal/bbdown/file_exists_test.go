package bbdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFileExistsPathUsesSharedSequenceForOutputGroup(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "视频.mp4")
	for _, path := range []string{original, filepath.Join(dir, "视频 (1).zh-CN.srt")} {
		if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolved, err := ResolveFileExistsPath(original, FileExistsActionRename)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "视频 (2).mp4")
	if resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestResolveFileExistsPathTreatsSidecarAsNameCollision(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "视频.mp4")
	if err := os.WriteFile(filepath.Join(dir, "视频.xml"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveFileExistsPath(original, FileExistsActionRename)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(dir, "视频 (1).mp4") {
		t.Fatalf("resolved path = %q", resolved)
	}
}

func TestResolveFileExistsPathInDirKeepsRelativeResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "视频.mp4"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveFileExistsPathInDir("视频.mp4", FileExistsActionRename, dir)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "视频 (1).mp4" {
		t.Fatalf("resolved path = %q", resolved)
	}
}

func TestResolveFileExistsPathInDirReservesNamesWithinOneTask(t *testing.T) {
	dir := t.TempDir()
	reserved := make(map[string]struct{})
	want := []string{"视频.mp4", "视频 (1).mp4", "视频 (2).mp4"}
	for index, expected := range want {
		resolved, err := resolveFileExistsPathInDir("视频.mp4", FileExistsActionRename, dir, reserved)
		if err != nil {
			t.Fatal(err)
		}
		if resolved != expected {
			t.Fatalf("resolved path %d = %q, want %q", index, resolved, expected)
		}
	}
}

func TestPrepareDownloadDestinationRemovesOnlyMatchingResumeFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "视频.mp4")
	remove := []string{
		path,
		path + ".aria2",
		singleThreadTempPath(path),
		filepath.Join(dir, "00000_视频.vclip"),
		filepath.Join(dir, "00001_视频.vclip"),
	}
	for _, candidate := range append(remove, filepath.Join(dir, "00000_其它.vclip")) {
		if err := os.WriteFile(candidate, []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := PrepareDownloadDestination(path, &MyOption{FileExistsAction: FileExistsActionOverwrite}); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range remove {
		if _, err := os.Stat(candidate); !os.IsNotExist(err) {
			t.Fatalf("resume file should be removed: %s (%v)", candidate, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "00000_其它.vclip")); err != nil {
		t.Fatalf("unrelated part should remain: %v", err)
	}
}

func TestFileExistsActionValidationAndAliases(t *testing.T) {
	for input, want := range map[string]string{
		"":          FileExistsActionSkip,
		"sequence":  FileExistsActionRename,
		"REPLACE":   FileExistsActionOverwrite,
		"overwrite": FileExistsActionOverwrite,
	} {
		if got := NormalizeFileExistsAction(input); got != want {
			t.Fatalf("NormalizeFileExistsAction(%q) = %q, want %q", input, got, want)
		}
	}
	if err := ValidateFileExistsAction("unknown"); err == nil {
		t.Fatal("invalid action should be rejected")
	}
}
