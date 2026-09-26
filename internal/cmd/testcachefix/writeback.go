package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// The cache keys and inputs below are computed the way cmd/go computes them
// in Go 1.27 (cmd/go/internal/test/test.go and cmd/go/internal/cache), so a
// written entry is one go itself would have written:
//
//   - a test result ID is the hash of "test binary <id> args <test args>
//     execcmd []", where id is the link action ID of the test binary for the
//     first level and its content ID for the second, both as `go tool
//     buildid` prints them; the test args are the -test.* flags go passes
//     the binary and keeps in the key, -test.v=test2json under -json and
//     -test.timeout;
//   - the result ID keys the test's input list, and the subkey
//     "inputs:<input ID>" of it keys the result for those inputs, where the
//     input ID hashes the list's environment variables, directory changes
//     and file stats the way computeTestInputsID does;
//   - every hash but the subkey is salted with the toolchain version.
//
// Any drift is caught, not guessed around: the second-level key computed
// here has to find the entry go wrote, or the package is skipped.

// cacheID is a cache key or output ID, 32 bytes as cmd/go's ActionID.
type cacheID [sha256.Size]byte

// cacheEntry is what an index file records: the output ID of the data file,
// its size and the time it was written.
type cacheEntry struct {
	output cacheID
	size   int64
	time   int64
}

// testlogMagic opens every test input list go stores.
var testlogMagic = []byte("# test log\n")

// modTimeCutoff is how recently written a file may be for its stat to stand
// in for its contents in the input ID; go refuses to cache a result that
// read a newer one, and so does the write-back.
const modTimeCutoff = 2 * time.Second

// writeback files the second-level result of every test binary the record
// directory holds under its first-level keys. Its flag -timeout is the
// -timeout go test ran with, part of the keys.
func writeback(args []string) error {
	flags := flag.NewFlagSet("writeback", flag.ContinueOnError)
	timeout := flags.String("timeout", "10m0s", "the -timeout go test ran with")
	if err := flags.Parse(args); err != nil {
		return errors.WithStack(err)
	}
	if flags.NArg() != 1 {
		usage()
	}
	records, err := readRecords(flags.Arg(0))
	if err != nil || len(records) == 0 {
		return err
	}
	cacheDir, err := goEnv("GOCACHE")
	if err != nil {
		return err
	}
	dirs, err := packageDirs(records)
	if err != nil {
		return err
	}
	testArgs := []string{"-test.v=test2json", "-test.timeout=" + *timeout}
	// A run that linked binaries it then ran, or whose inputs moved, files
	// nothing and says nothing: the count is reported when something was
	// filed or a binary was skipped for a reason worth reading.
	written, skipped := 0, 0
	for _, r := range records {
		dir, ok := dirs[r.importPath]
		if !ok {
			log.Printf("%s: not listed by go list, skipped", r.importPath)
			skipped++
			continue
		}
		buildID, err := binaryBuildID(r.binary)
		if err != nil {
			return errors.Wrapf(err, "%s", r.importPath)
		}
		done, err := writeBackOne(cacheDir, buildID, dir, testArgs)
		var skip skipError
		switch {
		case errors.As(err, &skip):
			log.Printf("%s: %s, skipped", r.importPath, skip)
			skipped++
		case err != nil:
			return errors.Wrapf(err, "%s", r.importPath)
		case done:
			written++
		}
	}
	if written > 0 || skipped > 0 {
		log.Printf("filed the results of %d of %d linked test binaries under their link keys", written, len(records))
	}
	return nil
}

// linkRecord is one line of the record: the package tested and the test
// binary kept for it.
type linkRecord struct {
	importPath string
	binary     string
}

// readRecords reads the record in the record directory; a missing record
// means no test binary was linked.
func readRecords(dir string) ([]linkRecord, error) {
	// #nosec G703 -- the record directory is the one make created for this run.
	f, err := os.Open(filepath.Join(dir, recordFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()
	var records []linkRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		importPath, name, ok := strings.Cut(scanner.Text(), "\t")
		if !ok {
			return nil, errors.Newf("%s: malformed line %q", f.Name(), scanner.Text())
		}
		records = append(records, linkRecord{importPath: importPath, binary: filepath.Join(dir, name)})
	}
	return records, errors.WithStack(scanner.Err())
}

// binaryBuildID reads the build ID go finalized in a test binary, as go
// itself prints it.
func binaryBuildID(binary string) (string, error) {
	// #nosec G702 -- binary is a test binary kept under the record directory.
	out, err := exec.Command("go", "tool", "buildid", binary).Output()
	if err != nil {
		return "", errors.Wrapf(err, "go tool buildid %s", binary)
	}
	return strings.TrimSpace(string(out)), nil
}

// skipError says why a binary's result was left as it was; writeback
// reports it and goes on to the next.
type skipError string

func (r skipError) Error() string { return string(r) }

// packageDir is where a package's tests ran and the module root go limits
// the input files it rechecks to.
type packageDir struct {
	dir  string
	root string
}

// packageDirs resolves the directory and root of every recorded package in
// one go list.
func packageDirs(records []linkRecord) (map[string]packageDir, error) {
	args := []string{"list", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{.Root}}"}
	for _, r := range records {
		args = append(args, r.importPath)
	}
	// #nosec G702 -- the import paths come from the linker's import configs.
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		return nil, errors.Wrap(err, "go list")
	}
	dirs := make(map[string]packageDir, len(records))
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, errors.Newf("go list: unexpected line %q", line)
		}
		dirs[fields[0]] = packageDir{dir: fields[1], root: fields[2]}
	}
	return dirs, nil
}

// goEnv returns the value of one go environment variable.
func goEnv(name string) (string, error) {
	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return "", errors.Wrapf(err, "go env %s", name)
	}
	return strings.TrimSpace(string(out)), nil
}

// writeBackOne files the second-level result of the test binary with
// buildID under its first-level keys and reports whether it did; a
// skipError says why it did not, any other error is a failure to read or
// write the cache.
func writeBackOne(cacheDir, buildID string, pkg packageDir, testArgs []string) (bool, error) {
	parts := strings.Split(buildID, "/")
	if len(parts) != 4 {
		return false, skipError(fmt.Sprintf("build ID %q has no link and content IDs", buildID))
	}
	linkID, contentID := testResultID(parts[0], testArgs), testResultID(parts[3], testArgs)
	inputList, ok, err := readEntry(cacheDir, contentID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, skipError("no result keyed by the binary's content")
	}
	// #nosec G703 -- the path is the cache's own, from the entry's output ID.
	testlog, err := os.ReadFile(dataPath(cacheDir, inputList.output))
	if err != nil {
		return false, errors.WithStack(err)
	}
	inputs, err := testInputsID(testlog, pkg.dir, pkg.root)
	if err != nil {
		return false, skipError(err.Error())
	}
	result, ok, err := readEntry(cacheDir, subkey(contentID, "inputs:"+hex.EncodeToString(inputs[:])))
	if err != nil {
		return false, err
	}
	if !ok {
		// The result go filed is for the inputs the test ran with, and they
		// have moved since — a test that reads what the run changes, such as
		// a walk of the repository — so there is nothing to file for the
		// inputs as they are now, and nothing to report: the next run runs
		// the test again either way.
		return false, nil
	}
	resultKey := subkey(linkID, "inputs:"+hex.EncodeToString(inputs[:]))
	if _, ok, err = readEntry(cacheDir, resultKey); err != nil || ok {
		return false, err
	}
	if err = writeEntry(cacheDir, linkID, inputList); err != nil {
		return false, err
	}
	return true, writeEntry(cacheDir, resultKey, result)
}

// testResultID is the key go files a test result under for the binary
// identified by id and the test args in the key.
func testResultID(id string, testArgs []string) cacheID {
	h := newHash()
	fmt.Fprintf(h, "test binary %s args %q execcmd %q", id, testArgs, []string(nil))
	return sum(h)
}

// subkey derives the key go uses for desc under parent.
func subkey(parent cacheID, desc string) cacheID {
	h := sha256.New()
	h.Write([]byte("subkey:"))
	h.Write(parent[:])
	h.Write([]byte(desc))
	return sum(h)
}

// newHash starts a hash the way cmd/go's cache does, salted with the
// toolchain version so a toolchain change keys everything afresh.
func newHash() hash.Hash {
	h := sha256.New()
	version := runtime.Version()
	if i := strings.Index(version, " X:"); i >= 0 {
		version = version[:i]
	}
	h.Write([]byte(version))
	return h
}

func sum(h hash.Hash) cacheID {
	var id cacheID
	copy(id[:], h.Sum(nil))
	return id
}

// testInputsID hashes a test's input list for its current state: the
// environment variables it read, the directories it changed into, and the
// files it stat'ed or opened inside the module root.
func testInputsID(testlog []byte, pkgDir, root string) (cacheID, error) {
	testlog = bytes.TrimPrefix(testlog, testlogMagic)
	h := newHash()
	// The runtime always reads GODEBUG, without saying so in the list.
	fmt.Fprintf(h, "env GODEBUG %x\n", hashGetenv("GODEBUG"))
	pwd := pkgDir
	for line := range bytes.SplitSeq(testlog, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		op, name, found := strings.Cut(string(line), " ")
		if !found {
			return cacheID{}, errors.Newf("input list malformed (%q)", line)
		}
		switch op {
		case "getenv":
			fmt.Fprintf(h, "env %s %x\n", name, hashGetenv(name))
		case "chdir":
			pwd = name // always absolute
			fmt.Fprintf(h, "chdir %s %x\n", name, hashStat(name))
		case "stat":
			if !filepath.IsAbs(name) {
				name = filepath.Join(pwd, name)
			}
			if root == "" || inDir(name, root) == "" {
				// Files outside the module are not rechecked.
				break
			}
			fmt.Fprintf(h, "stat %s %x\n", name, hashStat(name))
		case "open":
			if !filepath.IsAbs(name) {
				name = filepath.Join(pwd, name)
			}
			if root == "" || inDir(name, root) == "" {
				break
			}
			fh, err := hashOpen(name)
			if err != nil {
				return cacheID{}, errors.Wrapf(err, "input file %s", name)
			}
			fmt.Fprintf(h, "open %s %x\n", name, fh)
		default:
			return cacheID{}, errors.Newf("input list malformed (%q)", line)
		}
	}
	return sum(h), nil
}

func hashGetenv(name string) cacheID {
	h := newHash()
	v, ok := os.LookupEnv(name)
	if !ok {
		h.Write([]byte{0})
	} else {
		h.Write([]byte{1})
		h.Write([]byte(v))
	}
	return sum(h)
}

// hashOpen hashes the stat of an opened file, and of every entry of an
// opened directory; a regular file written less than modTimeCutoff ago is
// refused, since its stat may not yet reflect its contents.
func hashOpen(name string) (cacheID, error) {
	h := newHash()
	info, err := os.Stat(name)
	if err != nil {
		fmt.Fprintf(h, "err %v\n", err)
		return sum(h), nil
	}
	hashWriteStat(h, info)
	if info.IsDir() {
		files, err := os.ReadDir(name)
		if err != nil {
			fmt.Fprintf(h, "err %v\n", err)
		}
		for _, f := range files {
			fmt.Fprintf(h, "file %s ", f.Name())
			finfo, err := f.Info()
			if err != nil {
				fmt.Fprintf(h, "err %v\n", err)
			} else {
				hashWriteStat(h, finfo)
			}
		}
	} else if info.Mode().IsRegular() && time.Since(info.ModTime()) < modTimeCutoff {
		return cacheID{}, errors.New("file used as input is too new")
	}
	return sum(h), nil
}

func hashStat(name string) cacheID {
	h := newHash()
	if info, err := os.Stat(name); err != nil {
		fmt.Fprintf(h, "err %v\n", err)
	} else {
		hashWriteStat(h, info)
	}
	if info, err := os.Lstat(name); err != nil {
		fmt.Fprintf(h, "err %v\n", err)
	} else {
		hashWriteStat(h, info)
	}
	return sum(h)
}

func hashWriteStat(h hash.Hash, info fs.FileInfo) {
	fmt.Fprintf(h, "stat %d %x %v %v\n", info.Size(), uint64(info.Mode()), info.ModTime(), info.IsDir())
}

// inDir reports whether path is in the tree rooted at dir, as
// cmd/go/internal/search.InDir does: lexically first, then through the
// symbolic links of either.
func inDir(path, dir string) string {
	if rel, ok := inDirLex(path, dir); ok {
		return rel
	}
	xpath, err := filepath.EvalSymlinks(path)
	if err != nil || xpath == path {
		xpath = ""
	} else if rel, ok := inDirLex(xpath, dir); ok {
		return rel
	}
	xdir, err := filepath.EvalSymlinks(dir)
	if err == nil && xdir != dir {
		if rel, ok := inDirLex(path, xdir); ok {
			return rel
		}
		if xpath != "" {
			if rel, ok := inDirLex(xpath, xdir); ok {
				return rel
			}
		}
	}
	return ""
}

func inDirLex(path, dir string) (string, bool) {
	if dir == "" {
		return path, true
	}
	switch {
	case len(path) == len(dir):
		if path != dir {
			return "", false
		}
		return ".", true
	case len(path) > len(dir):
		if dir[len(dir)-1] == filepath.Separator {
			if !strings.HasPrefix(path, dir) {
				return "", false
			}
			return path[len(dir):], true
		}
		if path[len(dir)] != filepath.Separator || path[:len(dir)] != dir {
			return "", false
		}
		return path[len(dir)+1:], true
	}
	return "", false
}

// indexPath and dataPath are where the cache keeps the index entry of a key
// and the data of an output ID.
func indexPath(cacheDir string, id cacheID) string {
	name := hex.EncodeToString(id[:])
	return filepath.Join(cacheDir, name[:2], name+"-a")
}

func dataPath(cacheDir string, output cacheID) string {
	name := hex.EncodeToString(output[:])
	return filepath.Join(cacheDir, name[:2], name+"-d")
}

// indexEntryFormat is the index file: version, key, output ID, size and
// time, as putIndexEntry writes it.
const indexEntryFormat = "v1 %x %x %20d %20d\n"

// readEntry reads the index entry of id; ok is false when there is none or
// it does not carry id, which go treats as a miss.
func readEntry(cacheDir string, id cacheID) (cacheEntry, bool, error) {
	// #nosec G703 -- the path is the cache's own, from the key.
	raw, err := os.ReadFile(indexPath(cacheDir, id))
	if errors.Is(err, fs.ErrNotExist) {
		return cacheEntry{}, false, nil
	}
	if err != nil {
		return cacheEntry{}, false, errors.WithStack(err)
	}
	entry, ok := parseEntry(string(raw), id)
	return entry, ok, nil
}

// parseEntry parses an index file's line and reports whether it is one
// for id.
func parseEntry(raw string, id cacheID) (cacheEntry, bool) {
	var version, entryID, output string
	var entry cacheEntry
	if _, err := fmt.Sscanf(raw, "%s %s %s %d %d\n", &version, &entryID, &output, &entry.size, &entry.time); err != nil || version != "v1" {
		return cacheEntry{}, false
	}
	if entryID != hex.EncodeToString(id[:]) {
		return cacheEntry{}, false
	}
	out, err := hex.DecodeString(output)
	if err != nil || len(out) != len(entry.output) {
		return cacheEntry{}, false
	}
	copy(entry.output[:], out)
	return entry, true
}

// writeEntry files entry under id, the way putIndexEntry does.
func writeEntry(cacheDir string, id cacheID, entry cacheEntry) error {
	line := fmt.Sprintf(indexEntryFormat, id, entry.output, entry.size, entry.time)
	if err := os.MkdirAll(filepath.Dir(indexPath(cacheDir, id)), 0o750); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.WriteFile(indexPath(cacheDir, id), []byte(line), 0o600))
}
