// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The content ID below is computed the way cmd/internal/buildid of Go 1.27
// computes it, FindAndHash in rewrite.go and HashToString in buildid.go, with
// the Mach-O load commands read through debug/macho; that package is internal
// to cmd and cannot be imported from outside it.

package main

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"io"
	"os"

	"github.com/cockroachdb/errors"
)

// The Mach-O load commands whose data the hash leaves out.
const (
	lcUUID          = 0x1b
	lcCodeSignature = 0x1d
)

// The Mach-O file header sizes, which the load commands follow.
const (
	machoHeaderSize32 = 28
	machoHeaderSize64 = 32
)

// contentID computes the content ID of the binary at path: the hash of the
// file with every occurrence of buildID, the one go gave the linker, and the
// host's own build ID replaced by zeros and the Mach-O code signature left
// out, as go prints it in the last part of the finalized build ID.
func contentID(path, buildID string) (string, error) {
	// #nosec G703 -- path is the linker's own -o under go's work directory.
	f, err := os.Open(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer f.Close()
	hash, err := findAndHash(f, buildID)
	if err != nil {
		return "", err
	}
	return hashToString(hash), nil
}

// findAndHash reads all of f and returns a hash of its content with the
// occurrences of id replaced by zeros.
func findAndHash(f *os.File, id string) (hash [32]byte, err error) {
	const bufSize = 31 * 1024 // bufSize+little will likely fit in 32 kB
	if len(id) == 0 {
		return hash, errors.New("no build ID to find")
	}
	if len(id) > bufSize {
		return hash, errors.New("build ID longer than the buffer")
	}
	zeros := make([]byte, len(id))
	idBytes := []byte(id)

	var r io.Reader = f

	// For Mach-O files, we want to exclude the code signature.
	// The code signature contains hashes of the whole file (except the signature
	// itself), including the buildid. So the buildid cannot contain the signature.
	r = excludeMachoCodeSignature(r, f)

	// With the "-B gobuildid" linker option (which will be the default on some
	// platforms), the host build ID (GNU build ID, Mach-O UUID) depends on the
	// Go buildid. So ignore the host build ID, to avoid convergence problem.
	r = excludeHostBuildID(r, f)

	// The strategy is to read the file through buf, looking for id,
	// but we need to worry about what happens if id is broken up
	// and returned in parts by two different reads.
	// We allocate a tiny buffer (at least len(id)) and a big buffer (bufSize bytes)
	// next to each other in memory and then copy the tail of
	// one read into the tiny buffer before reading new data into the big buffer.
	// The search for id is over the entire tiny+big buffer.
	tiny := (len(id) + 127) &^ 127 // round up to 128-aligned
	buf := make([]byte, tiny+bufSize)
	h := sha256.New()
	start := tiny
	for {
		// buf[start:tiny] is left over from previous iteration.
		// After reading n bytes into buf[tiny:], we process buf[start:tiny+n].
		n, err := io.ReadFull(r, buf[tiny:])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return hash, errors.WithStack(err)
		}

		// Process any matches.
		for {
			i := bytes.Index(buf[start:tiny+n], idBytes)
			if i < 0 {
				break
			}
			h.Write(buf[start : start+i])
			h.Write(zeros)
			start += i + len(id)
		}
		if n < bufSize {
			// Did not fill buffer, must be at end of file.
			h.Write(buf[start : tiny+n])
			break
		}

		// Process all but final tiny bytes of buf (bufSize = len(buf)-tiny).
		// Note that start > len(buf)-tiny is possible, if the search above
		// found an id ending in the final tiny fringe. That's OK.
		if start < len(buf)-tiny {
			h.Write(buf[start : len(buf)-tiny])
			start = len(buf) - tiny
		}

		// Slide ending tiny-sized fringe to beginning of buffer.
		copy(buf[0:], buf[bufSize:])
		start -= bufSize
	}
	h.Sum(hash[:0])
	return hash, nil
}

// excludeMachoCodeSignature wraps r to read the Mach-O code signature of the
// file at ra as zeros; a file that is not Mach-O, or carries no signature, is
// read as it is.
func excludeMachoCodeSignature(r io.Reader, ra io.ReaderAt) io.Reader {
	f, err := macho.NewFile(ra)
	if err != nil {
		return r
	}
	for _, l := range f.Loads {
		data := l.Raw()
		if len(data) >= 16 && f.ByteOrder.Uint32(data) == lcCodeSignature {
			dataoff, datasize := f.ByteOrder.Uint32(data[8:]), f.ByteOrder.Uint32(data[12:])
			return &excludedReader{r, 0, int64(dataoff), int64(dataoff + datasize)}
		}
	}
	return r
}

// excludeHostBuildID wraps r to read the host build ID of the file at ra as
// zeros, see findHostBuildID.
func excludeHostBuildID(r io.Reader, ra io.ReaderAt) io.Reader {
	off, sz, ok := findHostBuildID(ra)
	if !ok {
		return r
	}
	return &excludedReader{r, 0, off, off + sz}
}

// excludedReader wraps an io.Reader. Reading from it returns the bytes from
// the underlying reader, except that when the byte offset is within the
// range between start and end, it returns zero bytes.
type excludedReader struct {
	r          io.Reader
	off        int64 // current offset
	start, end int64 // the range to be excluded (read as zero)
}

func (r *excludedReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 && r.off+int64(n) > r.start && r.off < r.end {
		cstart := max(r.start-r.off, 0)
		cend := min(r.end-r.off, int64(n))
		clear(p[cstart:cend])
	}
	r.off += int64(n)
	return n, err
}

// findHostBuildID returns where the host build ID of the file at ra is: the
// GNU build ID note of an ELF file, or the UUID of a Mach-O file.
func findHostBuildID(ra io.ReaderAt) (offset, size int64, ok bool) {
	if ef, err := elf.NewFile(ra); err == nil {
		sect := ef.Section(".note.gnu.build-id")
		if sect == nil {
			return 0, 0, false
		}
		// Skip over the 3-word note "header" and "GNU\x00".
		return int64(sect.Offset + 16), int64(sect.Size - 16), true //nolint:gosec
	}

	mf, err := macho.NewFile(ra)
	if err != nil {
		return 0, 0, false
	}
	// The load commands follow the file header, each at the offset the ones
	// before it add up to; the UUID is the data of the LC_UUID command,
	// past its 8-byte header.
	offset = machoHeaderSize32
	if mf.Magic == macho.Magic64 {
		offset = machoHeaderSize64
	}
	for _, l := range mf.Loads {
		data := l.Raw()
		if len(data) >= 8 && mf.ByteOrder.Uint32(data) == lcUUID {
			return offset + 8, int64(len(data) - 8), true
		}
		offset += int64(len(data))
	}
	return 0, 0, false
}

// hashToString returns the first 120 bits of h, five chunks of 24 bits, as
// 20 characters of go's base64 alphabet, the form build IDs are printed in.
func hashToString(h [32]byte) string {
	const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	const chunks = 5
	var dst [chunks * 4]byte
	for i := range chunks {
		v := uint32(h[3*i])<<16 | uint32(h[3*i+1])<<8 | uint32(h[3*i+2])
		dst[4*i+0] = b64[(v>>18)&0x3F]
		dst[4*i+1] = b64[(v>>12)&0x3F]
		dst[4*i+2] = b64[(v>>6)&0x3F]
		dst[4*i+3] = b64[v&0x3F]
	}
	return string(dst[:])
}
