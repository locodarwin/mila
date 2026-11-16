// Package ids makes the IDs Mila works out rather than draws: the same
// inputs make the same ID on every device and on the server, so two
// devices that make the same row offline make one row.
package ids

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

// HashID is a UUID-shaped (version 8, RFC 9562) SHA-256 of a string.
func HashID(s string) string {
	sum := sha256.Sum256([]byte(s))
	b := sum[:16]
	b[6] = b[6]&0x0f | 0x80
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Pair is the ID of a row that stands for a pair (a label on a task, a
// person assigned to it): a hash of the two IDs in order, so nobody can
// choose two of their own that make another's, as they could when it
// was the two XORed (XOR, kept for what was made that way).
func Pair(a, b string) string {
	if a == "" || b == "" {
		return ""
	}
	return HashID(a + "/" + b)
}

// XOR is the two UUIDs XORed, "" when either is not one: how pair IDs
// were made before Pair, and still how a wiki's main page's ID is made
// from its project's or workspace's, which links in documents name.
func XOR(a, b string) string {
	x, errX := uuid.Parse(a)
	y, errY := uuid.Parse(b)
	if errX != nil || errY != nil {
		return ""
	}
	var out uuid.UUID
	for i := range out {
		out[i] = x[i] ^ y[i]
	}
	return out.String()
}

// Shape is what kind of ID a string is, as far as its form says.
type Shape int

const (
	// Other is anything that is not a UUID written the usual way
	// (lowercase, with dashes), which no ID Mila works out can equal.
	Other Shape = iota
	// Drawn is a UUID of versions 1 to 7 (RFC 9562), what devices and
	// scripts draw.
	Drawn
	// Hashed is a version 8 UUID, the shape of HashID.
	Hashed
	// Odd is any other UUID: an XOR of two (a main page's ID) is never
	// of the RFC's variant.
	Odd
)

// ShapeOf says which Shape id has.
func ShapeOf(id string) Shape {
	u, err := uuid.Parse(id)
	if err != nil || u.String() != id {
		return Other
	}
	switch v := u.Version(); {
	case u.Variant() != uuid.RFC4122:
		return Odd
	case v == 8:
		return Hashed
	case v >= 1 && v <= 7:
		return Drawn
	}
	return Odd
}
