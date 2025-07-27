package cmd

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	set "github.com/deckarep/golang-set/v2"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list [path]",
		Short:   "List all tags",
		Aliases: []string{"ls", "l"},
		Args:    cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				p, exist := os.LookupEnv("OBSIDIAN_VAULT_PATH")
				if !exist {
					return fmt.Errorf("path not provided and OBSIDIAN_VAULT_PATH is not set")
				}
				args = append(args, p)
			}

			root, err := newVaultPath(args[0])
			if err != nil {
				return err
			}

			isIgnored, err := loadIgnoredTags(root)
			if err != nil {
				return err
			}

			ns, err := listNotes(root)
			if err != nil {
				return err
			}

			var tc tagCounts

			// try to read cache
			tc, err = newTagCountsFromCache(root)
			// if cache is valid and no changes was detected, return it
			if err == nil && tc.Hash == ns.hash {
				fmt.Printf("%s\n", tc)
				return nil
			}

			// cache is stale, corrupted, or missing, compute tag counts
			tc = newTagCounts(ns, isIgnored)
			// write computed tag counts to cache
			if err := tc.writeCache(root); err != nil {
				// failing to write cache is not a fatal error, just log it
				log.Printf("failed to write cache to %s: %v", root.cachePath(), err)
			}

			fmt.Printf("%s\n", tc)

			return nil
		},
	}

	return cmd
}

type tagCounts struct {
	Tags map[string]int `json:"tags"`
	Hash uint64         `json:"hash"`
}

func newTagCounts(ns noteSet, ignoredTags set.Set[string]) tagCounts {
	tags := collectTags(ns.notes).Difference(ignoredTags)

	m := make(map[string]int, tags.Cardinality())
	for t := range set.Elements(tags) {
		m[t]++
	}
	return tagCounts{
		Tags: m,
		Hash: ns.hash,
	}
}

func newTagCountsFromCache(root vaultPath) (tagCounts, error) {
	var tc tagCounts

	dataFile := root.cachePath()
	f, err := os.Open(dataFile)
	if err != nil {
		return tc, err
	}
	d := json.NewDecoder(f)
	if err := d.Decode(&tc); err != nil {
		return tc, err
	}
	return tc, nil
}

func (tc tagCounts) writeCache(root vaultPath) error {
	f, err := os.OpenFile(root.cachePath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")
	if err := enc.Encode(tc); err != nil {
		return err
	}
	return nil
}

// TODO: clean up this function
func (tc tagCounts) String() string {
	b, err := json.MarshalIndent(tc, "", "\t")
	if err != nil {
		panic(err)
	}
	return string(b)
}

// loadIgnoredTags reads the '.tobiignore' file at root directory, which contains
// tag names to ignore, one per line. Empty lines are skipped and duplicate entries
// are removed.
//
// Returns an empty set if the file doesn't exist or cannot be read due to permissions.
//
// Returns an error for other file system issues.
func loadIgnoredTags(root vaultPath) (set.Set[string], error) {
	lines := set.NewSet[string]()
	ignoreFile := root.ignorePath()

	b, err := os.ReadFile(ignoreFile)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return lines, nil
		case errors.Is(err, fs.ErrPermission):
			log.Printf("permission denied to read %s", ignoreFile)
			return lines, nil
		default:
			return nil, err
		}
	}

	for l := range strings.Lines(string(b)) {
		l = strings.TrimSuffix(l, "\n")
		if l == "" {
			continue
		}
		lines.Add(l)
	}

	return lines, nil
}

// collectTags processes all note files concurrently and extracts tags from their
// YAML frontmatter, returning a deduplicated set of all discovered tags.
//
// Files that cannot be processed due to errors are logged and skipped.
func collectTags(notes set.Set[string]) set.Set[string] {
	var wg sync.WaitGroup

	ch := make(chan []string, notes.Cardinality())

	for n := range set.Elements(notes) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tags, err := processFile(n)
			if err != nil {
				log.Printf("failed to process file %s: %v", n, err)
				ch <- nil
				return
			}
			ch <- tags
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	allTags := set.NewSetWithSize[string](1024)
	for tags := range ch {
		allTags.Append(tags...)
	}
	return allTags
}

// processFile opens a file and extracts tags from its YAML frontmatter.
// Returns nil (without error) if the file has no frontmatter or empty frontmatter.
//
// Returns an error if the file cannot be opened, frontmatter is invalid, or YAML parsing fails.
func processFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	yml, err := extractFrontMatter(f)
	if errors.Is(err, ErrEmptyFrontMatter) || errors.Is(err, ErrNoFrontMatter) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return extractTagsFromYAML([]byte(yml))
}

var (
	ErrInvalidFrontMatter = errors.New("invalid frontmatter")
	ErrEmptyFrontMatter   = errors.New("empty frontmatter")
	ErrNoFrontMatter      = errors.New("no frontmatter")
)

// extractFrontMatter reads from the given reader and extracts YAML frontmatter
// content enclosed between '---' delimiters, returning the frontmatter as a string.
//
// Returns an error if delimiters are missing or frontmatter is empty.
func extractFrontMatter(r io.Reader) (string, error) {
	sep := "---"

	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		t := scanner.Text()
		if !strings.HasPrefix(t, sep) {
			return "", ErrNoFrontMatter
		}
		if t != sep {
			return "", ErrInvalidFrontMatter
		}
	}

	var (
		s   strings.Builder
		end bool
	)

	for scanner.Scan() {
		t := scanner.Text()
		if t == sep {
			end = true
			break
		}
		s.WriteString(t + "\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if !end {
		return "", ErrInvalidFrontMatter
	}

	yml := strings.TrimSpace(s.String())
	if len(yml) == 0 {
		return "", ErrEmptyFrontMatter
	}

	return yml, nil
}

// extractTagsFromYAML parses YAML frontmatter data and extracts the "tags" field,
// returning the tags as a slice of strings.
//
// Returns an error if the YAML is invalid.
func extractTagsFromYAML(data []byte) ([]string, error) {
	var fm struct {
		Tags []string `yaml:"tags"`
	}

	if err := yaml.Unmarshal(data, &fm); err != nil {
		return nil, err
	}

	for i := range fm.Tags {
		fm.Tags[i] = strings.TrimPrefix(fm.Tags[i], "#")
	}

	return fm.Tags, nil
}

// vaultPath is a path to a valid directory.
type vaultPath string

func newVaultPath(path string) (vaultPath, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return vaultPath(path), nil
}

func (v vaultPath) String() string {
	return string(v)
}

func (v vaultPath) ignorePath() string {
	return filepath.Join(v.String(), ".tobiignore")
}

func (v vaultPath) cachePath() string {
	return filepath.Join(v.String(), ".tobi.json")
}

// noteSet represents a collection of discovered note files with cache validation.
// The hash field is calculated from file paths and modification times to detect
// changes in the vault for cache invalidation.
type noteSet struct {
	notes set.Set[string]
	hash  uint64
}

// listNotes recursively traverses the directory at root and discovers all '.md' files
// that should be tracked, filtering out files ignored by .gitignore patterns and
// skipping the .git directory. It returns a noteSet containing the discovered files
// and a hash calculated from file paths and modification times for cache validation.
//
// Files that cannot be accessed for file info are logged and skipped.
//
// Returns an error if the root path is invalid or .gitignore patterns cannot be read.
func listNotes(root vaultPath) (noteSet, error) {
	h := fnv.New64a()
	m, err := newGitIgnoredMatcher(root)
	if err != nil {
		return noteSet{}, err
	}

	notes := set.NewSet[string]()
	err = filepath.WalkDir(root.String(), func(path string, d fs.DirEntry, err error) error {
		// Skip directory entry if there's an error
		if err != nil {
			return nil
		}

		// Skip .git directory
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.Type().IsRegular() && filepath.Ext(path) == ".md" {
			// matchFile will returns an error if the path can't be made relative to root.
			// However, this is not possible in WalkDir, so ignoring error is safe.
			skip, _ := m.matchFile(path)
			if skip {
				return nil
			}

			info, err := d.Info()
			// Skip files where we can't get info. Info() returns fs.ErrNotExist if the file
			// has been removed or renamed since the directory read. Since we're only reading
			// (not modifying files), this should never happen. However, we log the error
			// as a safeguard to warn anyone against accidentally modifying files during traversal.
			if err != nil {
				log.Printf("failed to get file info for %s: %v", path, err)
				return nil
			}

			h.Write([]byte(path))
			binary.Write(h, binary.LittleEndian, info.ModTime().Unix())

			notes.Add(path)
		}

		return nil
	})
	if err != nil {
		return noteSet{}, err
	}

	return noteSet{
		notes: notes,
		hash:  h.Sum64(),
	}, nil
}

// gitignoreMatcher wraps a gitignore.Matcher with root directory context
// to enable matching files by their absolute paths against .gitignore patterns.
type gitignoreMatcher struct {
	gitignore.Matcher
	root vaultPath
}

// newGitIgnoredMatcher creates a gitignoreMatcher by reading .gitignore patterns
// from the specified root directory and its subdirectories recursively.
//
// Returns an error if .gitignore patterns cannot be read.
func newGitIgnoredMatcher(root vaultPath) (gitignoreMatcher, error) {
	rfs := osfs.New(root.String(), osfs.WithBoundOS())

	ps, err := gitignore.ReadPatterns(rfs, nil)
	if err != nil {
		return gitignoreMatcher{}, err
	}

	return gitignoreMatcher{
		gitignore.NewMatcher(ps),
		root,
	}, nil
}

// matchFile checks if an absolute file path should be ignored based on .gitignore patterns.
// Converts the absolute path to a path relative to the root directory before matching.
//
// Returns an error if the absolute path cannot be made relative to the root directory.
func (m *gitignoreMatcher) matchFile(absPath string) (bool, error) {
	relPath, err := filepath.Rel(m.root.String(), absPath)
	if err != nil {
		return false, err
	}
	s := strings.Split(relPath, string(filepath.Separator))
	return m.Match(s, false), nil
}
