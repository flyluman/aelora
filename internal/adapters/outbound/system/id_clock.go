package system

import (
	"crypto/rand"
	"encoding/base32"
	"sync"
	"time"
)

var crockford = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

type IDGenerator struct {
	mu      sync.Mutex
	lastMS  int64
	entropy [10]byte
}

func NewIDGenerator() *IDGenerator { return &IDGenerator{} }

func (g *IDGenerator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	ms := time.Now().UTC().UnixMilli()
	if ms != g.lastMS {
		g.lastMS = ms
		_, _ = rand.Read(g.entropy[:])
	} else {
		incrementEntropy(&g.entropy)
	}

	var raw [16]byte
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], g.entropy[:])

	return crockford.EncodeToString(raw[:])
}

func incrementEntropy(entropy *[10]byte) {
	for i := len(entropy) - 1; i >= 0; i-- {
		entropy[i]++
		if entropy[i] != 0 {
			return
		}
	}
}

type Clock struct{}

func NewClock() Clock { return Clock{} }

func (Clock) Now() time.Time { return time.Now() }
