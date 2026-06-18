package queue

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryQueue is the ITER-0000 in-memory stub substrate. It implements the full
// Queue contract (atomic claim + lease + requeue + not-before eligibility) so
// the daemon written against it works unchanged when laneq replaces it.
//
// Safe for concurrent use.
type MemoryQueue struct {
	mu      sync.Mutex
	now     func() time.Time
	seq     int
	pending []*Directive          // eligible/deferred, awaiting claim
	claimed map[string]*claimItem // by directive ID
}

type claimItem struct {
	d     *Directive
	token string
	exp   time.Time
}

// NewMemoryQueue returns a stub queue using the wall clock.
func NewMemoryQueue() *MemoryQueue { return NewMemoryQueueWithClock(time.Now) }

// NewMemoryQueueWithClock returns a stub queue using the supplied clock — used
// by tests to drive lease expiry and not-before deterministically.
func NewMemoryQueueWithClock(now func() time.Time) *MemoryQueue {
	return &MemoryQueue{now: now, claimed: map[string]*claimItem{}}
}

func (q *MemoryQueue) newID() string {
	q.seq++
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("d-%d-%s", q.seq, hex.EncodeToString(b[:]))
}

func newToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Push enqueues a directive, assigning an ID if empty.
func (q *MemoryQueue) Push(d Directive) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if d.ID == "" {
		d.ID = q.newID()
	}
	if d.Importance == "" {
		d.Importance = ImportanceNormal
	}
	cp := d
	q.pending = append(q.pending, &cp)
	return d.ID, nil
}

// Claim atomically reserves the highest-priority eligible pending directive.
func (q *MemoryQueue) Claim(consumer string, leaseDur time.Duration) (Directive, Lease, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()

	best := -1
	for i, d := range q.pending {
		if !d.NotBefore.IsZero() && d.NotBefore.After(now) {
			continue // not yet eligible
		}
		if best == -1 || less(d, q.pending[best]) {
			best = i
		}
	}
	if best == -1 {
		return Directive{}, Lease{}, ErrEmpty
	}

	d := q.pending[best]
	q.pending = append(q.pending[:best], q.pending[best+1:]...)

	lease := Lease{DirectiveID: d.ID, Token: newToken(), Expiry: now.Add(leaseDur)}
	q.claimed[d.ID] = &claimItem{d: d, token: lease.Token, exp: lease.Expiry}
	return *d, lease, nil
}

// less reports whether a should be claimed before b: lower effective priority
// first, ties broken by earlier-pushed (FIFO via insertion order is preserved
// because we scan in slice order and only replace on strict improvement).
func less(a, b *Directive) bool {
	return priorityOf(a.Importance) < priorityOf(b.Importance)
}

func (q *MemoryQueue) live(lease Lease) (*claimItem, error) {
	it, ok := q.claimed[lease.DirectiveID]
	if !ok || it.token != lease.Token {
		return nil, ErrLeaseLost
	}
	return it, nil
}

// Touch renews a lease.
func (q *MemoryQueue) Touch(lease Lease, leaseDur time.Duration) (Lease, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	it, err := q.live(lease)
	if err != nil {
		return Lease{}, err
	}
	it.exp = q.now().Add(leaseDur)
	return Lease{DirectiveID: lease.DirectiveID, Token: it.token, Expiry: it.exp}, nil
}

// Done marks a claimed directive complete.
func (q *MemoryQueue) Done(lease Lease) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, err := q.live(lease); err != nil {
		return err
	}
	delete(q.claimed, lease.DirectiveID)
	return nil
}

// Requeue returns a claimed directive to pending, incrementing Attempts.
func (q *MemoryQueue) Requeue(lease Lease, notBefore time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	it, err := q.live(lease)
	if err != nil {
		return err
	}
	delete(q.claimed, lease.DirectiveID)
	it.d.Attempts++
	it.d.NotBefore = notBefore
	q.pending = append(q.pending, it.d)
	return nil
}

// Reap reclaims expired leases by requeueing them.
func (q *MemoryQueue) Reap() (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	var expired []string
	for id, it := range q.claimed {
		if !it.exp.After(now) {
			expired = append(expired, id)
		}
	}
	sort.Strings(expired) // deterministic
	for _, id := range expired {
		it := q.claimed[id]
		delete(q.claimed, id)
		it.d.Attempts++
		q.pending = append(q.pending, it.d)
	}
	return len(expired), nil
}

// Len reports pending and claimed counts.
func (q *MemoryQueue) Len() (int, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending), len(q.claimed)
}
