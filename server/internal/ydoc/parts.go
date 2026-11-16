package ydoc

import "errors"

// maxParts is how many structs and delete ranges one update may hold. In
// memory each costs far more than its bytes (a delete range of three bytes
// takes about 300), so an update of a few megabytes could take hundreds;
// a person's edits, a paste of a long page included, hold far fewer.
const maxParts = 100_000

// ErrTooManyParts means an update holds more parts than maxParts.
var ErrTooManyParts = errors.New("ydoc: update has too many parts")

// countParts counts a V1 update's structs and delete ranges without
// building any of them. It reads only as far as it needs: an update it
// cannot follow is left to the full decoder to judge.
func countParts(update []byte) (int, bool) {
	r := partReader{b: update}
	n := 0
	groups := r.uint()
	for range groups {
		structs := r.uint()
		r.uint() // client
		r.uint() // clock
		if n += int(structs); n > maxParts || r.bad {
			return n, !r.bad
		}
		for range structs {
			info := r.byte()
			switch info & 0x1f {
			case 0, 10: // GC, skip
				r.uint()
			default:
				r.item(info)
			}
			if r.bad {
				return n, false
			}
		}
	}
	clients := r.uint()
	for range clients {
		r.uint() // client
		ranges := r.uint()
		if n += int(ranges); n > maxParts || r.bad {
			return n, !r.bad
		}
		for range ranges {
			r.uint()
			r.uint()
		}
	}
	return n, !r.bad
}

type partReader struct {
	b   []byte
	at  int
	bad bool
}

func (r *partReader) byte() byte {
	if r.at >= len(r.b) {
		r.bad = true
		return 0
	}
	c := r.b[r.at]
	r.at++
	return c
}

// uint reads a lib0 varuint (a varint is skipped the same way).
func (r *partReader) uint() uint64 {
	var v uint64
	for shift := 0; shift < 64; shift += 7 {
		c := r.byte()
		v |= uint64(c&0x7f) << shift
		if c < 0x80 || r.bad {
			return v
		}
	}
	r.bad = true
	return 0
}

func (r *partReader) skip(n uint64) {
	if n > uint64(len(r.b)-r.at) {
		r.bad = true
		return
	}
	r.at += int(n)
}

func (r *partReader) str() { r.skip(r.uint()) }

func (r *partReader) item(info byte) {
	if info&0x80 != 0 { // origin
		r.uint()
		r.uint()
	}
	if info&0x40 != 0 { // right origin
		r.uint()
		r.uint()
	}
	if info&0xc0 == 0 {
		if r.uint() == 1 {
			r.str() // parent's name
		} else {
			r.uint() // parent's ID
			r.uint()
		}
		if info&0x20 != 0 {
			r.str() // parentSub
		}
	}
	switch info & 0x1f {
	case 1: // deleted
		r.uint()
	case 2: // JSON
		for range r.uint() {
			r.str()
		}
	case 3, 4, 5: // binary, string, embed
		r.str()
	case 6: // format
		r.str()
		r.str()
	case 7: // type
		if ref := r.uint(); ref == 3 || ref == 5 {
			r.str()
		}
	case 8: // any
		for range r.uint() {
			r.any(0)
		}
	case 9: // doc
		r.str()
		r.any(0)
	default:
		r.bad = true
	}
}

// any skips one lib0 "any" value.
func (r *partReader) any(depth int) {
	if depth > 64 {
		r.bad = true
		return
	}
	switch r.byte() {
	case 127, 126, 121, 120: // undefined, null, false, true
	case 125: // varint
		r.uint()
	case 124:
		r.skip(4)
	case 123, 122:
		r.skip(8)
	case 119, 116: // string, bytes
		r.str()
	case 118: // object
		for range r.uint() {
			r.str()
			r.any(depth + 1)
			if r.bad {
				return
			}
		}
	case 117: // array
		for range r.uint() {
			r.any(depth + 1)
			if r.bad {
				return
			}
		}
	default:
		r.bad = true
	}
}
