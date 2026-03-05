package application

import (
	"context"
	"sync/atomic"
	"time"
)

const (
	roomIDWidth  = 8
	roomIDSpace  = uint64(218340105584896) // 62^8
	roomHalfBase = uint32(14776336)        // 62^4
)

var roomIDAlphabet = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

const defaultRoomIDMask = uint64(0x7a4bd9f1c3e5)

type RoomIDSequence interface {
	Next(ctx context.Context) (uint64, error)
}

type SequentialRoomIDGenerator struct {
	sequence RoomIDSequence
	mask     uint64
}

type localRoomIDSequence struct {
	counter atomic.Uint64
}

func NewLocalRoomIDSequence(seed uint64) RoomIDSequence {
	seq := &localRoomIDSequence{}
	seq.counter.Store(seed)
	return seq
}

func (s *localRoomIDSequence) Next(_ context.Context) (uint64, error) {
	return s.counter.Add(1), nil
}

func NewSequentialRoomIDGenerator(sequence RoomIDSequence, xorMask uint64) *SequentialRoomIDGenerator {
	if sequence == nil {
		sequence = NewLocalRoomIDSequence(uint64(time.Now().UTC().UnixNano()))
	}
	mask := xorMask % uint64(roomHalfBase)
	if mask == 0 {
		mask = defaultRoomIDMask % uint64(roomHalfBase)
	}
	return &SequentialRoomIDGenerator{sequence: sequence, mask: mask}
}

func (g *SequentialRoomIDGenerator) New(ctx context.Context) (string, error) {
	n, err := g.sequence.Next(ctx)
	if err != nil {
		return "", err
	}
	seq := (n - 1) % roomIDSpace
	obfuscated := g.feistelObfuscate(seq)
	return encodeBase62Fixed(obfuscated, roomIDWidth), nil
}

func (g *SequentialRoomIDGenerator) feistelObfuscate(seq uint64) uint64 {
	left := uint32(seq / uint64(roomHalfBase))
	right := uint32(seq % uint64(roomHalfBase))

	for round := range uint64(6) {
		nextLeft := right
		roundValue := g.round(right, round)
		right = (left + roundValue) % roomHalfBase
		left = nextLeft
	}

	return uint64(left)*uint64(roomHalfBase) + uint64(right)
}

func (g *SequentialRoomIDGenerator) round(right uint32, round uint64) uint32 {
	x := uint64(right)
	x ^= g.mask
	x ^= (round + 1) * 0x9e3779b185ebca87
	x ^= x >> 17
	x *= 0xff51afd7ed558ccd
	x ^= x >> 15
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 16
	return uint32(x % uint64(roomHalfBase))
}

func encodeBase62Fixed(value uint64, width int) string {
	if width <= 0 {
		return ""
	}
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = roomIDAlphabet[value%62]
		value /= 62
	}
	return string(buf)
}
