package client

import (
	"net"
	"sync"
	"time"

	"github.com/pion/stun"
)

const (
	maxRtxInterval time.Duration = 1600 * time.Millisecond
)

// TransactionResult is a bag of result values of a transaction
type TransactionResult struct {
	Msg     *stun.Message
	From    net.Addr
	Retries int
	Err     error
}

// TransactionConfig is a set of confi params used by NewTransaction
type TransactionConfig struct {
	Key      string
	Raw      []byte
	To       net.Addr
	Interval time.Duration
}

// Transaction represents a transaction
type Transaction struct {
	Key      string                 // read-only
	Raw      []byte                 // read-only
	To       net.Addr               // read-only
	nRtx     int                    // modified only by the timer thread
	interval time.Duration          // modified only by the timer thread
	timer    *time.Timer            // therad-safe, set only by the creator, and stopper
	resultCh chan TransactionResult // thread-safe
	mutex    sync.RWMutex
}

// NewTransaction creates a new instance of Transaction
func NewTransaction(config *TransactionConfig) *Transaction {
	return &Transaction{
		Key:      config.Key,                   // read-only
		Raw:      config.Raw,                   // read-only
		To:       config.To,                    // read-only
		interval: config.Interval,              // modified only by the timer thread
		resultCh: make(chan TransactionResult), // thread-safe
	}
}

// StartRtxTimer starts the transaction timer
func (t *Transaction) StartRtxTimer(onTimeout func(trKey string, nRtx int)) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.timer = time.AfterFunc(t.interval, func() {
		t.nRtx++
		t.interval *= 2
		if t.interval > maxRtxInterval {
			t.interval = maxRtxInterval
		}
		onTimeout(t.Key, t.nRtx)
	})
}

// StopRtxTimer stop the transaction timer
func (t *Transaction) StopRtxTimer() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.timer != nil {
		t.timer.Stop()
	}
}

// WriteResult writes the result to the result channel
func (t *Transaction) WriteResult(res TransactionResult) bool {
	select {
	case t.resultCh <- res:
		return true
	default:
	}
	return false
}

// WaitForResult waits for the transaction result
func (t *Transaction) WaitForResult() TransactionResult {
	return <-t.resultCh
}

// Close closes the transaction
func (t *Transaction) Close() {
	close(t.resultCh)
}

// Retries returns the number of retransmission it has made
func (t *Transaction) Retries() int {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.nRtx
}

////////////////////////////////////////////////////////////////////////////////

// TransactionMap is a thread-safe transaction map
type TransactionMap struct {
	trMap map[string]*Transaction
	mutex sync.RWMutex
}

// NewTransactionMap create a new instance of the transaction map
func NewTransactionMap() *TransactionMap {
	return &TransactionMap{
		trMap: map[string]*Transaction{},
	}
}

// Insert inserts a trasaction to the map
func (m *TransactionMap) Insert(key string, tr *Transaction) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.trMap[key] = tr
	return true
}

// Find looks up a transaction by its key
func (m *TransactionMap) Find(key string) (*Transaction, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	tr, ok := m.trMap[key]
	return tr, ok
}

// Delete deletes a transaction by its key
func (m *TransactionMap) Delete(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.trMap, key)
}

// CloseAndDeleteAll closes and deletes all transactions
func (m *TransactionMap) CloseAndDeleteAll() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for trKey, tr := range m.trMap {
		tr.Close()
		delete(m.trMap, trKey)
	}
}

// Size returns the length of the transaction map
func (m *TransactionMap) Size() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.trMap)
}
