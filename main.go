package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jnewth/goutil"
)

const (
	defaultDocURL = "https://cad.onshape.com/documents/12312312345abcabcabcdeff/w/a855e4161c814f2e9ab3698a"
	apiBase       = "https://cad.onshape.com/api/v14"
)

var (
	fsVersionRe     = regexp.MustCompile(`(?m)^(FeatureScript )[\d.]+;`)
	importVersionRe = regexp.MustCompile(`, version : "[\d.]+"`)
)

type Settings struct {
	AccessKey  string `json:"accessKey"`
	SecretKey  string `json:"secretKey"`
	UseProxy   bool   `json:"useProxy"`
	URL        string `json:"URL"`
	ProxyKey   string `json:"proxyKey"`
	OnshapeKey string `json:"onshapeKey"`
}

type Version struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Microversion string `json:"microversion"`
	CreatedAt    string `json:"createdAt"`
}

type Element struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ElementType string `json:"elementType"`
	DataType    string `json:"dataType"`
}

type FeatureStudioResponse struct {
	Contents string `json:"contents"`
}

type ImportEntry struct {
	Version   string `toml:"version"`
	Retrieved string `toml:"retrieved"`
}

type ImportLog struct {
	Entry []ImportEntry `toml:"entry"`
}

func loadSettings(filename string) (Settings, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Settings{}, fmt.Errorf("could not read settings file: %w", err)
	}
	var s Settings
	if err = json.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("invalid JSON in settings file: %w", err)
	}
	if s.URL == "" {
		return Settings{}, fmt.Errorf("url missing (e.g. https://cad.onshape.com/api/v14)")
	}
	if s.UseProxy {
		if s.OnshapeKey == "" {
			return Settings{}, fmt.Errorf("onshapeKey missing (required when useProxy is true)")
		}
		if s.ProxyKey == "" {
			return Settings{}, fmt.Errorf("proxyKey missing (required when useProxy is true)")
		}
	} else {
		if s.AccessKey == "" {
			return Settings{}, fmt.Errorf("accessKey missing (required when useProxy is false)")
		}
		if s.SecretKey == "" {
			return Settings{}, fmt.Errorf("secretKey missing (required when useProxy is false)")
		}
	}
	return s, nil
}

func apiGet(s Settings, endpoint string, params url.Values) []byte {
	resolvedEndpoint := strings.Replace(endpoint, apiBase, s.URL, 1)

	fullURL := resolvedEndpoint
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}
	req, err := http.NewRequest("GET", fullURL, nil)
	goutil.Verify(err == nil, "failed to build request: %v", err)
	req.Header.Set("Accept", "application/json;charset=UTF-8; qs=0.09")
	if s.UseProxy {
		req.Header.Set("Authorization", "Basic "+s.OnshapeKey)
		req.Header.Set("ReframeApiKey", s.ProxyKey)
	} else {
		req.SetBasicAuth(s.AccessKey, s.SecretKey)
	}
	resp, err := http.DefaultClient.Do(req)
	goutil.Verify(err == nil, "request failed: %v", err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	goutil.Verify(err == nil, "failed to read response body: %v", err)
	goutil.Verify(resp.StatusCode >= 200 && resp.StatusCode < 300,
		"request to %s returned HTTP %d: %s", fullURL, resp.StatusCode, string(body))
	return body
}

var docIDRe = regexp.MustCompile(`documents/(\w+)`)

func parseOnshapePath(rawURL string) string {
	m := docIDRe.FindStringSubmatch(rawURL)
	goutil.Verify(len(m) == 2, "failed to extract document ID from URL: %s", rawURL)
	return m[1]
}

func getLatestVersion(s Settings, docID string) Version {
	endpoint := fmt.Sprintf("%s/documents/d/%s/versions", apiBase, docID)
	body := apiGet(s, endpoint, url.Values{})
	var versions []Version
	err := json.Unmarshal(body, &versions)
	goutil.Verify(err == nil, "failed to parse versions response: %v", err)
	goutil.Verify(len(versions) > 0, "no versions found for document %s", docID)
	return versions[len(versions)-1]
}

const importLogHeader = "# Onshape Standard Library import history.\n# Maintained by os-std-importer; do not edit manually.\n\n"

func readLatestImportedVersion(outDir string) string {
	cmd := exec.Command("git", "show", "with-versions:import-log.toml")
	cmd.Dir = outDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var log ImportLog
	if _, err := toml.Decode(string(out), &log); err != nil || len(log.Entry) == 0 {
		return ""
	}
	return log.Entry[len(log.Entry)-1].Version
}

func appendImportLog(outDir, version, retrieved string) {
	path := filepath.Join(outDir, "import-log.toml")
	var log ImportLog
	if data, err := os.ReadFile(path); err == nil {
		toml.Decode(string(data), &log) //nolint: errcheck — missing file is handled above
	}
	log.Entry = append(log.Entry, ImportEntry{Version: version, Retrieved: retrieved})
	var buf bytes.Buffer
	buf.WriteString(importLogHeader)
	err := toml.NewEncoder(&buf).Encode(log)
	goutil.Verify(err == nil, "failed to encode import-log.toml: %v", err)
	err = os.WriteFile(path, buf.Bytes(), 0644)
	goutil.Verify(err == nil, "failed to write import-log.toml: %v", err)
}

func listElements(s Settings, docID, versionID string) []Element {
	endpoint := fmt.Sprintf("%s/documents/d/%s/v/%s/elements", apiBase, docID, versionID)
	params := url.Values{}
	params.Set("withThumbnails", "false")
	body := apiGet(s, endpoint, params)
	var elements []Element
	err := json.Unmarshal(body, &elements)
	goutil.Verify(err == nil, "failed to parse elements response: %v", err)
	return elements
}

func getFeatureStudioSource(s Settings, docID, versionID, eid string) string {
	endpoint := fmt.Sprintf("%s/featurestudios/d/%s/v/%s/e/%s", apiBase, docID, versionID, eid)
	body := apiGet(s, endpoint, url.Values{})
	var resp FeatureStudioResponse
	err := json.Unmarshal(body, &resp)
	goutil.Verify(err == nil, "failed to parse feature studio response: %v", err)
	return resp.Contents
}

func downloadElements(s Settings, docID, outDir, versionID string, verbose bool) {
	existing, err := filepath.Glob(filepath.Join(outDir, "*.fs"))
	goutil.Verify(err == nil, "failed to glob .fs files: %v", err)
	for _, f := range existing {
		goutil.Verify(os.Remove(f) == nil, "failed to delete %s", f)
	}

	elements := listElements(s, docID, versionID)
	for _, el := range elements {
		if !strings.EqualFold(el.ElementType, "FEATURESTUDIO") {
			continue
		}
		goutil.Verboseln(verbose, el.Name)
		contents := getFeatureStudioSource(s, docID, versionID, el.ID)
		path := filepath.Join(outDir, strings.TrimSuffix(el.Name, ".fs")+".fs")
		err := os.WriteFile(path, []byte(contents), 0644)
		goutil.Verify(err == nil, "failed to write %s: %v", path, err)
	}
}

func stripVersions(outDir string) {
	files, err := filepath.Glob(filepath.Join(outDir, "*.fs"))
	goutil.Verify(err == nil, "failed to glob .fs files: %v", err)
	for _, f := range files {
		data, err := os.ReadFile(f)
		goutil.Verify(err == nil, "failed to read %s: %v", f, err)
		content := string(data)
		content = fsVersionRe.ReplaceAllString(content, `${1}; /** without versions **/`)
		content = importVersionRe.ReplaceAllString(content, `, version : ""`)
		err = os.WriteFile(f, []byte(content), 0644)
		goutil.Verify(err == nil, "failed to write %s: %v", f, err)
	}
}

func gitRun(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	goutil.Verify(err == nil, "git %s failed: %v", strings.Join(args, " "), err)
}

func commitWithVersions(outDir, versionName, date string) {
	gitRun(outDir, "add", "-A")
	gitRun(outDir, "commit", "-m", fmt.Sprintf("version: %s retrieved: %s", versionName, date))
}

func commitWithoutVersions(outDir, versionName, date string) {
	gitRun(outDir, "checkout", "without-versions")
	gitRun(outDir, "checkout", "with-versions", "--", ".")
	stripVersions(outDir)
	gitRun(outDir, "add", "-A")
	gitRun(outDir, "commit", "-m", fmt.Sprintf("version: %s retrieved: %s without-versions", versionName, date))
}

func main() {
	settingsFlag := flag.String("settings", "", "")
	outFlag := flag.String("out", "", "")
	dryRunFlag := flag.Bool("d", false, "")
	verboseFlag := flag.Bool("v", false, "")
	docURLFlag := flag.String("onshape-doc-url", "", "")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: os-std-importer --settings=<file> --out=<dir> [-d] [-v] [--onshape-doc-url=<url>]\n\n")
		fmt.Fprintf(os.Stderr, "Imports FeatureScript elements from an Onshape document into a local git repo.\n")
		fmt.Fprintf(os.Stderr, "Commits to the with-versions branch, then strips version strings and commits\n")
		fmt.Fprintf(os.Stderr, "to without-versions. Both branches must already exist in the output repo.\n\n")
		fmt.Fprintf(os.Stderr, "Required:\n")
		fmt.Fprintf(os.Stderr, "  --settings=<file>         Credentials/endpoint config; see remote.json.template\n")
		fmt.Fprintf(os.Stderr, "  --out=<dir>               Output git repo to commit into\n\n")
		fmt.Fprintf(os.Stderr, "Optional:\n")
		fmt.Fprintf(os.Stderr, "  -d                        Dry run: check version only, no download or commit\n")
		fmt.Fprintf(os.Stderr, "  -v                        Verbose: print each element name as fetched\n")
		fmt.Fprintf(os.Stderr, "  --onshape-doc-url=<url>   Target document (default: Onshape standard library)\n\n")
		fmt.Fprintf(os.Stderr, "Credential modes (useProxy field in settings file):\n")
		fmt.Fprintf(os.Stderr, "  false   accessKey + secretKey   direct Onshape API\n")
		fmt.Fprintf(os.Stderr, "  true    onshapeKey + proxyKey   via Reframe proxy (set proxyURL for endpoint)\n")
	}

	flag.Parse()

	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(0)
	}

	if *settingsFlag == "" {
		fmt.Fprintf(os.Stderr, "error: --settings is required\n\n")
		flag.Usage()
		os.Exit(1)
	}
	if *outFlag == "" {
		fmt.Fprintf(os.Stderr, "error: --out is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	docURL := defaultDocURL
	if *docURLFlag != "" {
		docURL = *docURLFlag
	}

	s, err := loadSettings(*settingsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading %q: %v\n", *settingsFlag, err)
		fmt.Fprintf(os.Stderr, "Copy remote.json.template to %s and fill in your credentials.\n\n", *settingsFlag)
		flag.Usage()
		os.Exit(1)
	}

	docID := parseOnshapePath(docURL)
	version := getLatestVersion(s, docID)
	last := readLatestImportedVersion(*outFlag)

	if last == version.Name {
		fmt.Println("Repo is at or ahead of Onshape document version")
		os.Exit(0)
	}

	if *dryRunFlag {
		fmt.Printf("New version available: %s\n", version.Name)
		os.Exit(0)
	}

	date := time.Now().Format("2006-01-02")
	gitRun(*outFlag, "checkout", "with-versions")
	downloadElements(s, docID, *outFlag, version.ID, *verboseFlag)
	appendImportLog(*outFlag, version.Name, date)
	commitWithVersions(*outFlag, version.Name, date)
	commitWithoutVersions(*outFlag, version.Name, date)
	fmt.Printf("Done. Committed version %s to both branches.\n", version.Name)
}
