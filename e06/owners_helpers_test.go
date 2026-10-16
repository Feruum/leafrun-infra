package e06

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	proof "github.com/leafrun/leafrun-contracts/ownerproof/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
	core "github.com/leafrun/leafrun-document-core"
)

const ownerNodeA = "http://127.0.0.1:58197"
const ownerNodeB = "http://127.0.0.1:58198"
const ownerProvider = "http://127.0.0.1:54433"
const ownerGeneration = "http://127.0.0.1:58290"
const ownerStorage = "http://127.0.0.1:58291"
const ownerProjects = "http://127.0.0.1:58296"
const ownerProjectManifest = `{"formatVersion":1,"name":"E06 current authority","engine":{"typstVersion":"0.15.1"},"templates":[{"id":"document","entrypoint":"templates/main.typ","inputSchema":"schemas/input.json"}],"targets":[{"id":"document","kind":"document","template":"document","output":{"format":"pdf","path":"output/result.pdf"}}],"profiles":{"default":{}},"defaultProfile":"default"}`
const ownerProjectSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"logo":{"type":"string","x-leafrun-file":true}},"additionalProperties":false}`
const ownerPrivateSentinel = "private-captured-source-e06"

func ownerEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("E06_OWNER_INTEGRATION") != "true" {
		t.Skip("E06_OWNER_INTEGRATION not enabled")
	}
	for _, key := range []string{"E06_KRATOS_ADMIN_URL", "E06_ACCESS_DATABASE_URL", "E06_PROJECTS_DATABASE_URL", "E06_STORAGE_DATABASE_URL", "E06_GENERATION_DATABASE_URL"} {
		if os.Getenv(key) == "" {
			t.Fatal("owned integration setting missing")
		}
	}
}
func ownerNonce(t *testing.T) string {
	t.Helper()
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		t.Fatal("fixture random failed")
	}
	return hex.EncodeToString(data[:])
}
func ownerGrantOperationKey(t *testing.T) string {
	t.Helper()
	key, err := access.NewGrantOperationKey(time.Now().UTC())
	if err != nil {
		t.Fatal("grant fixture operation key creation failed")
	}
	return key
}
func ownerHTTP(jar http.CookieJar) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		mapping := map[string]string{"127.0.0.1:54433": "http://kratos:4433", "127.0.0.1:54444": "http://hydra:4444", "127.0.0.1:58197": "http://access-api-a:8080", "127.0.0.1:58198": "http://access-api-b:8080", "127.0.0.1:58199": "http://audit-api:8199", "127.0.0.1:58290": "http://generation-api:8080", "127.0.0.1:58291": "http://storage-api:8081", "127.0.0.1:58296": "http://projects-api:8084"}
		if target := mapping[address]; target != "" {
			u, _ := url.Parse(target)
			address = u.Host
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
	}
	return &http.Client{Transport: transport, Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func ownerRequest(t *testing.T, c *http.Client, method, target, token, origin, media string, in any) (int, []byte) {
	t.Helper()
	var data []byte
	switch value := in.(type) {
	case nil:
	case []byte:
		data = value
	default:
		var err error
		data, err = json.Marshal(in)
		if err != nil {
			t.Fatal("fixture request encoding failed")
		}
	}
	request, err := http.NewRequest(method, target, bytes.NewReader(data))
	if err != nil {
		t.Fatal("fixture request invalid")
	}
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if media != "" {
		request.Header.Set("Content-Type", media)
	}
	response, err := c.Do(request)
	if err != nil {
		t.Fatal("owned dependency request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 13<<20))
	if err != nil {
		t.Fatal("owned response read failed")
	}
	return response.StatusCode, body
}

type ownerHuman struct{ ID, Email, Password string }
type ownerSession struct{ Token, ID string }

func ownerIdentity(t *testing.T) ownerHuman {
	t.Helper()
	f := ownerHuman{Email: "owner-task5-" + ownerNonce(t) + "@e06.example.test", Password: ownerNonce(t) + "aA!"}
	status, body := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities", "", "", "application/json", map[string]any{"schema_id": "default", "state": "active", "traits": map[string]string{"email": f.Email}, "credentials": map[string]any{"password": map[string]any{"config": map[string]string{"password": f.Password}}}})
	var out struct {
		ID string `json:"id"`
	}
	if status != 201 || json.Unmarshal(body, &out) != nil || out.ID == "" {
		t.Fatal("owned fixture identity creation failed")
	}
	f.ID = out.ID
	t.Cleanup(func() {
		status, _ := ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+f.ID, "", "", "", nil)
		if status != 204 {
			t.Error("owned fixture identity cleanup failed")
		}
	})
	return f
}
func ownerLogin(t *testing.T, h ownerHuman) ownerSession {
	t.Helper()
	c := ownerHTTP(nil)
	status, body := ownerRequest(t, c, "GET", ownerProvider+"/self-service/login/api", "", "", "", nil)
	var flow struct {
		UI struct {
			Action string `json:"action"`
		} `json:"ui"`
	}
	if status != 200 || json.Unmarshal(body, &flow) != nil {
		t.Fatal("native provider flow failed")
	}
	status, body = ownerRequest(t, c, "POST", flow.UI.Action, "", "", "application/json", map[string]string{"method": "password", "identifier": h.Email, "password": h.Password})
	var out struct {
		Token   string `json:"session_token"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if status != 200 || json.Unmarshal(body, &out) != nil || out.Token == "" || out.Session.ID == "" {
		t.Fatal("native provider login failed")
	}
	return ownerSession{out.Token, out.Session.ID}
}
func ownerLogout(t *testing.T, s ownerSession) {
	t.Helper()
	status, _ := ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/sessions/"+s.ID, "", "", "", nil)
	if status != 204 {
		t.Fatal("exact fixture session revocation failed")
	}
}

type ownerPolicyRetry struct{ base http.RoundTripper }

func (r ownerPolicyRetry) RoundTrip(request *http.Request) (*http.Response, error) {
	for i := 0; ; i++ {
		response, err := r.base.RoundTrip(request)
		if err != nil || response.StatusCode != 503 || i >= 30 {
			return response, err
		}
		response.Body.Close()
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-time.After(100 * time.Millisecond):
		}
		copy := request.Clone(request.Context())
		if request.GetBody != nil {
			copy.Body, err = request.GetBody()
			if err != nil {
				return nil, err
			}
		}
		request = copy
	}
}
func ownerAccess(t *testing.T, node string) *access.SessionClient {
	t.Helper()
	c := ownerHTTP(nil)
	c.Transport = ownerPolicyRetry{c.Transport}
	out, err := access.NewSessionClientWithOrigin(node, "ory_kratos_session", ownerNodeA, true, c)
	if err != nil {
		t.Fatal("Access public client configuration failed")
	}
	return out
}

type ownerPoolKey struct {
	t        *testing.T
	key, dsn string
}

var ownerPools sync.Map

func ownerPool(t *testing.T, key string) *pgxpool.Pool {
	t.Helper()
	cacheKey := ownerPoolKey{t, key, os.Getenv(key)}
	if saved, ok := ownerPools.Load(cacheKey); ok {
		return saved.(*pgxpool.Pool)
	}
	config, err := pgxpool.ParseConfig(cacheKey.dsn)
	if err != nil {
		t.Fatal("owned catalog configuration failed")
	}
	config.MaxConns = 2
	p, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal("owned catalog connection failed")
	}
	if p.Ping(context.Background()) != nil {
		p.Close()
		t.Fatal("owned catalog connection failed")
	}
	if saved, loaded := ownerPools.LoadOrStore(cacheKey, p); loaded {
		p.Close()
		return saved.(*pgxpool.Pool)
	}
	t.Cleanup(func() {
		ownerPools.Delete(cacheKey)
		p.Close()
	})
	return p
}
func ownerBind(t *testing.T, c *access.SessionClient, auth access.ResolveRequest, scope access.PermissionScope, subject, relation string, remove bool) {
	t.Helper()
	if _, err := c.Bind(context.Background(), auth, scope, access.BindingRequest{SubjectKind: "human", SubjectID: subject, Relation: relation}, remove); err != nil {
		t.Fatalf("owned %s binding failed", relation)
	}
}
func ownerPermission(t *testing.T, session ownerSession, scope storage.Scope, operation string, want bool) {
	t.Helper()
	c, err := access.NewClient(ownerNodeA, os.Getenv("E06_PROJECTS_SERVICE_TOKEN"), ownerHTTP(nil))
	if err != nil {
		t.Fatal("owned permission client invalid")
	}
	sessionClient := ownerAccess(t, ownerNodeA)
	resolved, err := sessionClient.Session(context.Background(), access.ResolveRequest{Bearer: session.Token})
	if err != nil {
		t.Fatal("owned current caller resolution failed")
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		allowed, checkErr := c.Check(context.Background(), access.CheckRequest{Principal: resolved.Identity.Principal, Authority: &resolved.Identity.Authority, Scope: access.PermissionScope{WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}, Operation: operation})
		if checkErr == nil && allowed == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("current %s permission did not converge to %v", operation, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
	c, err = access.NewClient(ownerNodeB, os.Getenv("E06_PROJECTS_SERVICE_TOKEN"), ownerHTTP(nil))
	if err != nil {
		t.Fatal("owned node B checker invalid")
	}
	allowed, err := c.Check(context.Background(), access.CheckRequest{Principal: resolved.Identity.Principal, Authority: &resolved.Identity.Authority, Scope: access.PermissionScope{WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}, Operation: operation})
	if err != nil || allowed != want {
		t.Fatal("Access nodes disagree on current owner permission")
	}
}
func ownerResource(t *testing.T, c *storage.Client, scope storage.Scope, data []byte) storage.Resource {
	t.Helper()
	sum := sha256.Sum256(data)
	upload, err := c.CreateUpload(context.Background(), storage.CreateUploadRequest{Scope: scope, Class: storage.ClassResource, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(data)), IdempotencyKey: "owner-upload-" + ownerNonce(t)})
	if err != nil {
		t.Fatal("owned service upload failed")
	}
	if _, err = c.PutUpload(context.Background(), upload.UploadID, upload.WriterToken, bytes.NewReader(data)); err != nil {
		t.Fatal("owned service byte upload failed")
	}
	resource, err := c.CommitResource(context.Background(), upload.UploadID, storage.CommitResourceRequest{WriterToken: upload.WriterToken, IdempotencyKey: upload.UploadID})
	if err != nil {
		t.Fatal("owned resource commit failed")
	}
	return resource
}
func ownerSeedRevision(t *testing.T, p *pgxpool.Pool, scope storage.Scope, typst string) (storage.Snapshot, string) {
	t.Helper()
	ctx := context.Background()
	root, fonts := t.TempDir(), t.TempDir()
	for name, data := range map[string]string{"leafrun.json": ownerProjectManifest, "schemas/input.json": ownerProjectSchema, "templates/main.typ": typst, "private.txt": ownerPrivateSentinel} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if os.MkdirAll(filepath.Dir(path), 0700) != nil || os.WriteFile(path, []byte(data), 0600) != nil {
			t.Fatal("private source fixture write failed")
		}
	}
	privatePNG, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal("private image fixture invalid")
	}
	for name, data := range map[string][]byte{"private.json": []byte(`{"message":"private-captured-source-e06"}`), "private.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="blue"/></svg>`), "private.png": privatePNG} {
		if os.WriteFile(filepath.Join(root, name), data, 0600) != nil {
			t.Fatal("private captured asset write failed")
		}
	}
	font, err := os.ReadFile(os.Getenv("E06_FONT_FILE"))
	if err != nil || os.WriteFile(filepath.Join(fonts, "Fira.ttf"), font, 0600) != nil {
		t.Fatal("private captured font fixture unavailable")
	}
	captured, err := core.CreateSnapshot(ctx, core.FolderSource{Root: root}, core.SnapshotOptions{FontPaths: []string{fonts}})
	if err != nil {
		t.Fatal("Core capture failed")
	}
	defer captured.Close()
	manifest := captured.Manifest()
	wireData, _ := json.Marshal(manifest)
	var wire storage.SnapshotManifest
	if json.Unmarshal(wireData, &wire) != nil {
		t.Fatal("snapshot manifest conversion failed")
	}
	client, err := storage.NewClient(ownerStorage, os.Getenv("E06_GENERATION_STORAGE_TOKEN"), ownerHTTP(nil))
	if err != nil {
		t.Fatal("fixed storage client failed")
	}
	refs := []storage.ResourceRef{}
	send := func(tree fs.FS, files []core.SnapshotFile, kind string, index int) {
		for _, file := range files {
			data, err := fs.ReadFile(tree, file.Path)
			if err != nil {
				t.Fatal("captured resource unavailable")
			}
			resource := ownerResource(t, client, scope, data)
			refs = append(refs, storage.ResourceRef{Tree: kind, FontIndex: index, Path: file.Path, ResourceID: resource.ResourceID})
		}
	}
	send(captured.FS(), manifest.Files, "project", 0)
	for index, font := range manifest.Fonts {
		if !font.InProject {
			tree, err := captured.FontFS(index)
			if err != nil {
				t.Fatal("captured font unavailable")
			}
			send(tree, font.Files, "font", index)
		}
	}
	snapshot, err := client.CreateSnapshot(ctx, storage.CreateSnapshotRequest{Scope: scope, Manifest: wire, Resources: refs, IdempotencyKey: "owner-snapshot-" + ownerNonce(t)})
	if err != nil {
		t.Fatal("owned committed snapshot creation failed")
	}
	project, err := core.ReadProject(ctx, captured.FS())
	if err != nil {
		t.Fatal("Core project validation failed")
	}
	descriptors := []projects.TargetDescriptor{}
	for _, target := range project.Targets {
		descriptor, err := core.DescribeTarget(ctx, captured.FS(), project, manifest, target.ID, "")
		if err != nil {
			t.Fatal("Core target validation failed")
		}
		data, _ := json.Marshal(descriptor)
		var wire projects.TargetDescriptor
		if json.Unmarshal(data, &wire) != nil {
			t.Fatal("target descriptor conversion failed")
		}
		descriptors = append(descriptors, wire)
	}
	revision := "legacy-revision-" + ownerNonce(t)
	data, _ := json.Marshal(descriptors)
	// Test-only preexisting-domain setup. No public allow fixture or unverified
	// production bootstrap is introduced; import independently checks both owners.
	if _, err = p.Exec(ctx, `INSERT INTO projects_revisions(workspace_id,project_id,revision_id,snapshot_id,content_id,targets,metadata_bytes) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7)`, scope.WorkspaceID, scope.ProjectID, revision, snapshot.SnapshotID, snapshot.Manifest.ID, data, len(data)); err != nil {
		t.Fatal("preexisting Projects revision fixture failed")
	}
	return snapshot, revision
}
func ownerImport(t *testing.T, node, org string, scope storage.Scope, revision, owner string) (proof.ImportResult, error) {
	t.Helper()
	c, err := proof.NewClient(node, os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), ownerHTTP(nil))
	if err != nil {
		t.Fatal("operator client invalid")
	}
	return c.Import(context.Background(), proof.ImportRequest{OrganizationID: org, Scope: scope, Revision: revision, Kind: "personal", OwnerID: owner, IdempotencyKey: "claim-" + scope.WorkspaceID + "-" + scope.ProjectID})
}
func ownerRelease(t *testing.T, token string, scope storage.Scope, revision string) projects.Release {
	t.Helper()
	c, err := projects.NewClient(ownerProjects, token, ownerHTTP(nil))
	if err != nil {
		t.Fatal("Projects public client invalid")
	}
	release, err := c.CreateRelease(context.Background(), projects.CreateReleaseRequest{Scope: scope, Revision: revision, TargetID: "document", Version: "v-" + ownerNonce(t), IdempotencyKey: "release-" + ownerNonce(t)})
	if err != nil {
		t.Fatal("release admission failed")
	}
	deadline := time.Now().Add(25 * time.Second)
	for release.State != "ready" {
		if time.Now().After(deadline) {
			t.Fatal("original release pin activation did not complete")
		}
		time.Sleep(100 * time.Millisecond)
		release, err = c.GetRelease(context.Background(), release.ReleaseID)
		if err != nil {
			t.Fatal("release status failed")
		}
	}
	return release
}
func ownerJob(t *testing.T, c *generation.Client, id string) generation.Job {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		job, err := c.GetJob(context.Background(), id)
		if err != nil {
			t.Fatal("current job status failed")
		}
		if job.Status.Terminal() {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatal("actual Temporal render did not terminate")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func ownerRestricted(t *testing.T, job generation.Job) {
	t.Helper()
	if job.DiagnosticsVisibility != "restricted" || len(job.Diagnostics) != 0 || job.RawDiagnostics != "" || len(job.Parameters) != 0 || job.Entrypoint != "" {
		t.Fatal("restricted reader received source or selected-resource details")
	}
}
func ownerSessionIdentity(t *testing.T, session ownerSession) identity.Identity {
	t.Helper()
	out, err := ownerAccess(t, ownerNodeA).Session(context.Background(), access.ResolveRequest{Bearer: session.Token})
	if err != nil {
		t.Fatal("current session identity failed")
	}
	return out.Identity
}

func ownerCookieJar(t *testing.T, cookie string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(ownerNodeA)
	jar.SetCookies(u, []*http.Cookie{{Name: "ory_kratos_session", Value: cookie, Path: "/"}})
	return ownerHTTP(jar)
}
