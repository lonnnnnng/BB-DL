package bbdown

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMergeConfigArgsKeepsExplicitBoolValue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--skip-ai\ntrue\n--file-pattern\n<videoTitle>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "--skip-ai=false", "BV1xx"}, configPath)
	if slices.Contains(args, "--skip-ai") {
		t.Fatalf("config bool should not override explicit --skip-ai=false: %v", args)
	}
	if !slices.Contains(args, "--skip-ai=false") {
		t.Fatalf("explicit bool value missing: %v", args)
	}
	if !slices.Contains(args, "--file-pattern") {
		t.Fatalf("unrelated config value should still be merged: %v", args)
	}
}

func TestMergeConfigArgsSkipsValueWhenOptionOverridden(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--file-pattern\nfrom-config\n--download-danmaku\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "--file-pattern", "from-cli", "BV1xx"}, configPath)
	if slices.Contains(args, "from-config") {
		t.Fatalf("overridden config value should not become orphan positional arg: %v", args)
	}
	if !slices.Contains(args, "--download-danmaku") {
		t.Fatalf("unrelated config flag should still be merged: %v", args)
	}
}

func TestMergeConfigArgsSupportsFileExistsAction(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--file-exists-action\nrename\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := MergeConfigArgs([]string{"--config-file", configPath, "--file-exists-action", "overwrite", "BV1xx"}, configPath)
	if slices.Contains(args, "rename") || !slices.Contains(args, "overwrite") {
		t.Fatalf("explicit file action should override config: %v", args)
	}
}

func TestMergeConfigArgsNormalizesAliases(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--file-pattern\nfrom-config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "-F", "from-cli", "BV1xx"}, configPath)
	if slices.Contains(args, "from-config") || slices.Contains(args, "--file-pattern") {
		t.Fatalf("long config option should not override short cli alias: %v", args)
	}
}

func TestMergeConfigArgsBoolExplicitValue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--multi-thread\nfalse\n--skip-ai true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "BV1xx"}, configPath)
	for _, want := range []string{"--multi-thread", "false", "--skip-ai", "true"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
}

func TestMergeConfigArgsSingleLineQuotedValue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--file-pattern \"<videoTitle> [<dfn>]\"\n--download-danmaku\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "BV1xx"}, configPath)
	wantValue := "<videoTitle> [<dfn>]"
	for _, want := range []string{"--file-pattern", wantValue, "--download-danmaku"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
	if slices.Contains(args, "\""+wantValue+"\"") {
		t.Fatalf("quoted config value should be unwrapped: %v", args)
	}
}

func TestMergeConfigArgsInlineValueKeepsNextOption(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--file-pattern=from-config\n--download-danmaku\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "BV1xx"}, configPath)
	for _, want := range []string{"--file-pattern=from-config", "--download-danmaku"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
}

func TestMergeConfigArgsSkipsInlineValueWithoutDroppingNextOption(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "BBDown.config")
	if err := os.WriteFile(configPath, []byte("--file-pattern=from-config\n--download-danmaku\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := MergeConfigArgs([]string{"--config-file", configPath, "--file-pattern", "from-cli", "BV1xx"}, configPath)
	if slices.Contains(args, "--file-pattern=from-config") {
		t.Fatalf("inline config value should not override explicit CLI value: %v", args)
	}
	if !slices.Contains(args, "--download-danmaku") {
		t.Fatalf("next config option should not be dropped when inline value is skipped: %v", args)
	}
}
