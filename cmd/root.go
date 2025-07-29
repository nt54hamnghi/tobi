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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	set "github.com/deckarep/golang-set/v2"
	"github.com/goccy/go-yaml"
	"github.com/nt54hamnghi/tobi/pkg/gitignore"
	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag/v2"
)

type rootOptions struct {
	limit       int
	noCache     bool
	displayMode displayMode
}

func NewRootCmd() *cobra.Command {
	var opts rootOptions

	cmd := &cobra.Command{
		Use:   "tobi [path]",
		Short: "See all tags in your Obsidian vault",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(_ *cobra.Command, args []string) error {
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

			if !opts.noCache {
				// try to read cache
				tc, err = newTagCountsFromCache(root)
				// if cache is valid and no changes was detected, return it
				if err == nil && tc.Hash == ns.hash {
					tc.Print(opts)
					return nil
				}
			}

			// cache is disabled or cache file is stale, corrupted, or missing
			// compute tag counts
			tc = collectTags(ns, isIgnored)

			// write computed tag counts to cache
			if err := tc.writeCache(root); err != nil {
				// failing to write cache is not a fatal error, just log it
				log.Printf("failed to write cache to %s: %v", root.cachePath(), err)
			}

			tc.Print(opts)
			return nil
		},
	}

	flags := cmd.Flags()
	flags.SortFlags = false
	flags.IntVarP(&opts.limit, "limit", "l", 8, "number of tags to display. Negative values mean all.")
	flags.VarP(
		enumflag.New(&opts.displayMode, "mode", displayModeIDs, enumflag.EnumCaseSensitive),
		"mode", "m", "display mode (name|count)",
	)
	flags.BoolVarP(&opts.noCache, "no-cache", "n", false, "disable cache")

	// set up completion for display mode flag
	if err := cmd.RegisterFlagCompletionFunc("mode", completeDisplayModeFlag); err != nil {
		os.Exit(1)
	}

	return cmd
}

type tagCounts struct {
	Tags map[string]int `json:"tags"`
	Hash uint64         `json:"hash"`
}

// collectTags processes all note files concurrently and extracts tags from their
// YAML frontmatter, filtering out ignored tags and calculating frequency counts.
// Returns a tagCounts struct with the frequency map and vault hash.
//
// Files that cannot be processed due to errors are logged and skipped.
func collectTags(ns noteSet, ignoredTags set.Set[string]) tagCounts {
	var wg sync.WaitGroup

	// estimated total number of tags based on number of notes
	est := ns.notes.Cardinality() * 8
	// channel of tags to be collected
	ch := make(chan string, est)

	for n := range set.Elements(ns.notes) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tags, err := processFile(n)
			if err != nil {
				log.Printf("failed to process file %s: %v", n, err)
				return
			}
			for _, t := range tags {
				ch <- t
			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	m := make(map[string]int, est)
	for t := range ch {
		if ignoredTags.Contains(t) {
			continue
		}
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

func (tc tagCounts) Print(opts rootOptions) {
	t := slices.SortedFunc(maps.Keys(tc.Tags), func(a, b string) int {
		return tc.Tags[b] - tc.Tags[a]
	})

	var limit int
	if opts.limit < 0 {
		limit = len(t)
	} else {
		limit = min(len(t), opts.limit)
	}

	for i := 0; i < limit; i++ {
		tag := t[i]
		switch opts.displayMode {
		case name:
			fmt.Println(tag)
		case count:
			count := tc.Tags[tag]
			fmt.Printf("%s: %d\n", tag, count)
		}
	}
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

	absRoot, err := gitignore.NewAbsolutePath(string(root))
	if err != nil {
		return noteSet{}, err
	}

	m, err := gitignore.NewRepoRootMatcher(absRoot)
	if err != nil {
		return noteSet{}, err
	}

	notes := set.NewSet[string]()
	err = filepath.WalkDir(absRoot.String(), func(path string, d fs.DirEntry, err error) error {
		// Skip directory entry if there's an error
		if err != nil {
			return nil
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.Type().IsRegular() && filepath.Ext(path) == ".md" {
			// Since root is absolute when we pass it to WalkDir, path is absolute.
			// It's safe to construct AbsolutePath directly from path.
			skip := m.MatchFile(gitignore.NewAbsolutePathUnchecked(path))
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

			// TODO: these 2 calls return errors, might need to handle them
			_, _ = h.Write([]byte(path))
			_ = binary.Write(h, binary.LittleEndian, info.ModTime().Unix())

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
