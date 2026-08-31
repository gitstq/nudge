package store

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

// Monotonic, collision-resistant identifier: prefix + millisecond timestamp
// + process-wide counter + random bytes. Sortable by creation time.
var (
	idMu  sync.Mutex
	idSeq uint32
)

func newID(prefix string) string {
	idMu.Lock()
	idSeq = (idSeq + 1) & 0xffffff
	seq := idSeq
	now := time.Now().UnixMilli()
	idMu.Unlock()

	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return prefix + "_" + strconv.FormatInt(now, 36) + "_" +
		strconv.FormatUint(uint64(seq), 36) + "_" + hex.EncodeToString(b)
}
