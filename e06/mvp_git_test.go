package e06

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	storage "github.com/leafrun/leafrun-contracts/storage/v1"
	core "github.com/leafrun/leafrun-document-core"
	integrations "github.com/leafrun/leafrun-integrations"
)

type mvpGitSource struct{ source integrations.GitSource }

func (s mvpGitSource) Open(ctx context.Context) (core.SourceTree, error) {
	tree, err := s.source.Open(ctx)
	guards := make([]core.NativeProtection, len(tree.NativeProtection))
	for i, g := range tree.NativeProtection {
		guards[i] = core.NativeProtection{Path: g.Path, Info: g.Info}
	}
	o := tree.Origin
	return core.SourceTree{FS: tree.FS, Close: tree.Close, ProtectedPaths: tree.ProtectedPaths, NativeProtection: guards, Origin: core.SourceOrigin{Kind: o.Kind, Location: o.Location, SourceRoot: o.SourceRoot, Ref: o.Ref, Revision: o.Revision, ArchiveSHA256: o.ArchiveSHA256}}, err
}

func mvpGitSnapshot(t *testing.T, scope storage.Scope) (storage.Snapshot, string) {
	t.Helper()
	ctx := context.Background()
	origin, fonts := t.TempDir(), t.TempDir()
	files := map[string]string{
		"leafrun.json":       ownerProjectManifest,
		"schemas/input.json": `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"message":{"type":"string","minLength":1}},"required":["message"],"additionalProperties":false}`,
		"templates/main.typ": "#set text(font: \"Fira Sans\")\n#let p = json(sys.inputs.at(\"leafrun-data\"))\n#p.message\n#json(\"../data.json\").label\n#image(\"../art.svg\")",
		"data.json":          `{"label":"captured Git JSON"}`,
		"art.svg":            `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="blue"/></svg>`,
		"private.txt":        ownerPrivateSentinel,
	}
	for name, data := range files {
		target := filepath.Join(origin, filepath.FromSlash(name))
		if os.MkdirAll(filepath.Dir(target), 0700) != nil || os.WriteFile(target, []byte(data), 0600) != nil {
			t.Fatal("MVP Git fixture write failed")
		}
	}
	git := func(args ...string) string {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", origin}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_COUNT=0", "GIT_TERMINAL_PROMPT=0")
		data, err := command.Output()
		if err != nil {
			t.Fatal("MVP local Git fixture operation failed")
		}
		return strings.TrimSpace(string(data))
	}
	git("init", "--initial-branch=main")
	git("add", "--all")
	git("-c", "user.name=E06 fixture", "-c", "user.email=e06-fixture@example.invalid", "commit", "-m", "MVP source")
	commit := git("rev-parse", "HEAD")
	font, err := os.ReadFile(os.Getenv("E06_FONT_FILE"))
	if err != nil || os.WriteFile(filepath.Join(fonts, "Fira.ttf"), font, 0600) != nil {
		t.Fatal("MVP captured font fixture unavailable")
	}
	captured, err := core.CreateSnapshot(ctx, mvpGitSource{integrations.GitSource{URL: origin, Ref: commit}}, core.SnapshotOptions{FontPaths: []string{fonts}})
	if err != nil {
		t.Fatal("MVP actual Git capture failed")
	}
	defer captured.Close()
	manifest := captured.Manifest()
	if manifest.Origin.Kind != "git" || manifest.Origin.Revision != commit {
		t.Fatal("MVP Git provenance does not match committed origin")
	}
	bundle := core.SnapshotBundle{Manifest: manifest, ProjectFS: captured.FS(), ExternalFontFS: map[int]fs.FS{}}
	for i, font := range manifest.Fonts {
		if !font.InProject {
			tree, e := captured.FontFS(i)
			if e != nil {
				t.Fatal("MVP captured font tree unavailable")
			}
			bundle.ExternalFontFS[i] = tree
		}
	}
	// Remove only this fixture's known file and now-empty directory. Rendering
	// must subsequently use captured font bytes, never the original native path.
	if os.Remove(filepath.Join(fonts, "Fira.ttf")) != nil || os.Remove(fonts) != nil {
		t.Fatal("MVP original font removal failed")
	}
	restored, err := core.RestoreSnapshot(ctx, bundle, core.RestoreSnapshotOptions{})
	if err != nil {
		t.Fatal("MVP bundle restoration failed")
	}
	defer restored.Close()
	if restored.Manifest().ID != manifest.ID || restored.Manifest().Origin.Revision != commit {
		t.Fatal("MVP restored content ID/provenance changed")
	}
	client, err := storage.NewClient(ownerStorage, os.Getenv("E06_GENERATION_STORAGE_TOKEN"), ownerHTTP(nil))
	if err != nil {
		t.Fatal("MVP Storage integration client failed")
	}
	refs := []storage.ResourceRef{}
	send := func(tree fs.FS, files []core.SnapshotFile, kind string, index int) {
		for _, file := range files {
			data, e := fs.ReadFile(tree, file.Path)
			if e != nil {
				t.Fatal("MVP restored resource unavailable")
			}
			resource := ownerResource(t, client, scope, data)
			refs = append(refs, storage.ResourceRef{Tree: kind, FontIndex: index, Path: file.Path, ResourceID: resource.ResourceID})
		}
	}
	send(restored.FS(), manifest.Files, "project", 0)
	for i, font := range manifest.Fonts {
		if !font.InProject {
			tree, e := restored.FontFS(i)
			if e != nil {
				t.Fatal("MVP restored font unavailable")
			}
			send(tree, font.Files, "font", i)
		}
	}
	wireData, _ := json.Marshal(restored.Manifest())
	var wire storage.SnapshotManifest
	if json.Unmarshal(wireData, &wire) != nil {
		t.Fatal("MVP manifest conversion failed")
	}
	snapshot, err := client.CreateSnapshot(ctx, storage.CreateSnapshotRequest{Scope: scope, Manifest: wire, Resources: refs, IdempotencyKey: "mvp-snapshot-" + ownerNonce(t)})
	if err != nil || snapshot.Manifest.ID != manifest.ID {
		t.Fatal("MVP full captured snapshot commit failed")
	}
	return snapshot, commit
}
