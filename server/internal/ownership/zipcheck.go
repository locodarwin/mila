package ownership

import (
	"archive/zip"
	"bufio"
	"encoding/binary"
	"io"
	"math"
)

// Zip structure, as archive/zip reads it.
const (
	zipEndLen         = 22
	zipEnd64LocLen    = 20
	zipEnd64Len       = 56
	zipHeaderLen      = 46
	zipEndSig         = 0x06054b50
	zipEnd64LocSig    = 0x07064b50
	zipEnd64Sig       = 0x06064b50
	zipDirHeaderSig   = 0x02014b50
	zipEndSearchShort = 1024
	zipEndSearchLong  = 65 * 1024
)

// countEntries counts the records in a zip's central directory, stopping
// once there are more than limit, so a zip of millions of empty entries
// is refused before archive/zip builds a File for every one of them.
// archive/zip reads records until one does not parse, whatever count the
// zip states, so the count comes from the records. It finds the directory
// as archive/zip does, and where that could start in one of two places it
// counts from both and keeps the larger: never fewer than archive/zip
// would read.
func countEntries(r io.ReaderAt, size int64, limit int) (int, error) {
	end, rec, err := zipEnd(r, size)
	if err != nil {
		return 0, err
	}
	le := binary.LittleEndian
	records := uint64(le.Uint16(rec[10:]))
	dirSize := uint64(le.Uint32(rec[12:]))
	dirOffset := uint64(le.Uint32(rec[16:]))
	if records == 0xffff || dirSize == 0xffff || dirOffset == 0xffffffff {
		if loc := end - zipEnd64LocLen; loc >= 0 {
			b := make([]byte, zipEnd64LocLen)
			if _, err := r.ReadAt(b, loc); err != nil {
				return 0, err
			}
			if le.Uint32(b) == zipEnd64LocSig && le.Uint32(b[4:]) == 0 && le.Uint32(b[16:]) == 1 {
				at := int64(le.Uint64(b[8:]))
				e := make([]byte, zipEnd64Len)
				if _, err := r.ReadAt(e, at); err != nil {
					return 0, err
				}
				if le.Uint32(e) != zipEnd64Sig {
					return 0, zip.ErrFormat
				}
				end, dirSize, dirOffset = at, le.Uint64(e[40:]), le.Uint64(e[48:])
			}
		}
	}
	if dirSize > math.MaxInt64 || dirOffset > math.MaxInt64 {
		return 0, zip.ErrFormat
	}
	base := end - int64(dirSize) - int64(dirOffset)
	if o := base + int64(dirOffset); o < 0 || o >= size {
		return 0, zip.ErrFormat
	}
	most := countFrom(r, size, base+int64(dirOffset), limit)
	if base > 0 {
		most = max(most, countFrom(r, size, int64(dirOffset), limit))
	}
	return most, nil
}

// zipEnd finds a zip's end of central directory record: in its last
// kilobyte, or else its last 65.
func zipEnd(r io.ReaderAt, size int64) (int64, []byte, error) {
	for _, n := range []int64{zipEndSearchShort, zipEndSearchLong} {
		n = min(n, size)
		buf := make([]byte, n)
		if _, err := r.ReadAt(buf, size-n); err != nil && err != io.EOF {
			return 0, nil, err
		}
		if p := findZipEnd(buf); p >= 0 {
			return size - n + int64(p), buf[p:], nil
		}
		if n == size {
			break
		}
	}
	return 0, nil, zip.ErrFormat
}

func findZipEnd(b []byte) int {
	for i := len(b) - zipEndLen; i >= 0; i-- {
		if binary.LittleEndian.Uint32(b[i:]) == zipEndSig {
			comment := int(binary.LittleEndian.Uint16(b[i+zipEndLen-2:]))
			if comment+zipEndLen+i > len(b) {
				return -1
			}
			return i
		}
	}
	return -1
}

// countFrom counts the directory records that follow one another from
// start, up to one past limit.
func countFrom(r io.ReaderAt, size, start int64, limit int) int {
	if start < 0 || start >= size {
		return 0
	}
	br := bufio.NewReader(io.NewSectionReader(r, start, size-start))
	var h [zipHeaderLen]byte
	n := 0
	for n <= limit {
		if _, err := io.ReadFull(br, h[:]); err != nil {
			return n
		}
		le := binary.LittleEndian
		if le.Uint32(h[:]) != zipDirHeaderSig {
			return n
		}
		rest := int(le.Uint16(h[28:])) + int(le.Uint16(h[30:])) + int(le.Uint16(h[32:]))
		if d, err := br.Discard(rest); err != nil || d != rest {
			return n
		}
		n++
	}
	return n
}
